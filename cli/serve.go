package cli

import (
	"bytes"
	"crypto/sha1"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ije/gox/log"
	"github.com/ije/gox/utils"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type H struct {
	assets  *embed.FS
	loader  *LoaderWorker
	rootDir string
	wsConns map[*websocket.Conn]map[string]int64
	rwlock  sync.RWMutex
	lock    sync.RWMutex
	cache   sync.Map
}

func (h *H) ServeHtml(w http.ResponseWriter, r *http.Request, pathname string) {
	htmlFile, err := os.Open(filepath.Join(h.rootDir, pathname))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", 404)
		} else {
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}
	defer htmlFile.Close()
	tokenizer := html.NewTokenizer(htmlFile)
	unocssLink := ""
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken {
			token := tokenizer.Token()
			if token.DataAtom == atom.Script {
				var srcAttr string
				var mainAttr string
				for _, attr := range token.Attr {
					if attr.Key == "src" {
						srcAttr = attr.Val
					} else if attr.Key == "main" {
						mainAttr = attr.Val
					}
				}
				// add `im` query to the main script
				if isRelativeSpecifier(srcAttr) || strings.HasPrefix(srcAttr, "/") {
					w.Write([]byte("<script"))
					for _, attr := range token.Attr {
						if attr.Key == "src" {
							srcAttr, _ = utils.SplitByFirstByte(srcAttr, '?')
							w.Write([]byte(fmt.Sprintf(` src="%s?im=%s"`, srcAttr, btoaUrl(pathname))))
						} else {
							if attr.Val == "" {
								w.Write([]byte(fmt.Sprintf(` %s`, attr.Key)))
							} else {
								w.Write([]byte(fmt.Sprintf(` %s="%s"`, attr.Key, attr.Val)))
							}
						}
					}
					w.Write([]byte(">"))
					continue
				}
				// replace `<script type="module" src="https://esm.sh/run" main="$main"></script>`
				// with `<script type="module" src="$main"></script>`
				if (strings.HasPrefix(srcAttr, "http://") || strings.HasPrefix(srcAttr, "https://")) && strings.HasSuffix(srcAttr, "/run") && mainAttr != "" {
					w.Write([]byte("<script"))
					for _, attr := range token.Attr {
						if attr.Key != "main" {
							if attr.Key == "src" {
								mainAttr, _ = utils.SplitByFirstByte(mainAttr, '?')
								w.Write([]byte(fmt.Sprintf(` src="%s"`, mainAttr+"?im="+btoaUrl(pathname))))
							} else {
								if attr.Val == "" {
									w.Write([]byte(fmt.Sprintf(` %s`, attr.Key)))
								} else {
									w.Write([]byte(fmt.Sprintf(` %s="%s"`, attr.Key, attr.Val)))
								}
							}
						}
					}
					w.Write([]byte(">"))
					continue
				}
				// replace `<script src="https://esm.sh/uno"></script>`
				// with `<link rel="stylesheet" href="/@unocss">`
				if (strings.HasPrefix(srcAttr, "http://") || strings.HasPrefix(srcAttr, "https://")) && strings.HasSuffix(srcAttr, "/uno") {
					unocssLink = "/@unocss?ctx=" + btoaUrl(pathname)
					w.Write([]byte(fmt.Sprintf(`<link id="@unocss" rel="stylesheet" href="%s">`, unocssLink)))
					tok := tokenizer.Next()
					if tok == html.TextToken {
						tokenizer.Next()
					}
					if tok == html.ErrorToken {
						break
					}
					continue
				}
			} else if token.DataAtom == atom.Style {
				var typeAttr string
				for _, attr := range token.Attr {
					if attr.Key == "type" {
						typeAttr = attr.Val
						break
					}
				}
				// strip `<style type="uno/css">...</style>`
				if typeAttr == "uno/css" {
					tok := tokenizer.Next()
					if tok == html.TextToken {
						tokenizer.Next()
					}
					if tok == html.ErrorToken {
						break
					}
					continue
				}
			}
		}
		w.Write(tokenizer.Raw())
	}
	// reload the page when the html file is modified
	fmt.Fprintf(w, `<script type="module">import createHotContext from"/@hmr";const hot=createHotContext("%s");hot.watch(()=>location.reload());`, pathname)
	if unocssLink != "" {
		// reload the unocss when the module dependency tree is changed
		fmt.Fprintf(w, `hot.watch("*",(kind,filename)=>{if(/\.(jsx|tsx|vue|svelte)$/i.test(filename)){document.getElementById("@unocss").href="%s&t="+Date.now().toString(36)}})`, unocssLink)
	}
	w.Write([]byte("</script>"))
	fmt.Fprintf(w, `<script>console.log("%%c💚 Built with esm.sh/run, uncheck \"Disable cache\" in Network tab for better DX!", "color:green")</script>`)
}

func (h *H) ServeModule(w http.ResponseWriter, r *http.Request, pathname string) {
	im, err := atobUrl(r.URL.Query().Get("im"))
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	imHtmlFilename := filepath.Join(h.rootDir, im)
	imHtmlFile, err := os.Open(imHtmlFilename)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Bad Request", 400)
		} else {
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}
	defer imHtmlFile.Close()
	var importMap ImportMap
	tokenizer := html.NewTokenizer(imHtmlFile)
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken {
			token := tokenizer.Token()
			if token.DataAtom == atom.Script {
				var typeAttr string
				for _, attr := range token.Attr {
					if attr.Key == "type" {
						typeAttr = attr.Val
						break
					}
				}
				if typeAttr == "importmap" {
					tokenizer.Next()
					if json.Unmarshal(tokenizer.Text(), &importMap) != nil {
						header := w.Header()
						header.Set("Content-Type", "application/javascript; charset=utf-8")
						header.Set("Cache-Control", "max-age=0, must-revalidate")
						w.Write([]byte(`throw new Error("Failed to parse import map: invalid JSON")`))
						return
					}
					importMap.Src = "file://" + im
					break
				}
			} else if token.DataAtom == atom.Body {
				break
			}
		} else if tt == html.EndTagToken {
			tagName, _ := tokenizer.TagName()
			if bytes.Equal(tagName, []byte("head")) {
				break
			}
		}
	}
	code, err := os.ReadFile(filepath.Join(h.rootDir, pathname))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", 404)
		} else {
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}
	sha := sha1.New()
	sha.Write(code)
	etag := fmt.Sprintf("\"%x\"", sha.Sum(nil))
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	cacheKey := fmt.Sprintf("module-%s", pathname)
	etagCacheKey := fmt.Sprintf("module-%s.etag", pathname)
	if v, ok := h.cache.Load(cacheKey); ok {
		if e, ok := h.cache.Load(etagCacheKey); ok {
			if e.(string) == etag {
				header := w.Header()
				header.Set("Content-Type", "application/javascript; charset=utf-8")
				header.Set("Cache-Control", "max-age=0, must-revalidate")
				header.Set("Etag", etag)
				w.Write(v.([]byte))
				return
			}
		}
	}
	loader, err := h.getLoader()
	if err != nil {
		fmt.Println(log.Red(err.Error()))
		http.Error(w, "Internal Server Error", 500)
		return
	}
	js, err := loader.Load("module", pathname, importMap)
	if err != nil {
		fmt.Println(log.Red(err.Error()))
		http.Error(w, "Internal Server Error", 500)
		return
	}
	h.cache.Store(cacheKey, []byte(js))
	h.cache.Store(etagCacheKey, etag)
	header := w.Header()
	header.Set("Content-Type", "application/javascript; charset=utf-8")
	header.Set("Cache-Control", "max-age=0, must-revalidate")
	header.Set("Etag", etag)
	w.Write([]byte(js))
}

func (h *H) ServeUnoCSS(w http.ResponseWriter, r *http.Request) {
	ctx, err := atobUrl(r.URL.Query().Get("ctx"))
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	imHtmlFilename := filepath.Join(h.rootDir, ctx)
	imHtmlFile, err := os.Open(imHtmlFilename)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Bad Request", 400)
		} else {
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}
	defer imHtmlFile.Close()
	configCSS := ""
	tokenizer := html.NewTokenizer(imHtmlFile)
	input := []string{}
	jsEntries := map[string]struct{}{}
	importMap := ImportMap{}
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken {
			name, moreAttr := tokenizer.TagName()
			switch string(name) {
			case "style":
				for moreAttr {
					var key, val []byte
					key, val, moreAttr = tokenizer.TagAttr()
					if bytes.Equal(key, []byte("type")) && bytes.Equal(val, []byte("uno/css")) {
						tokenizer.Next()
						innerText := bytes.TrimSpace(tokenizer.Text())
						if len(innerText) > 0 {
							configCSS += string(innerText)
						}
						break
					}
				}
			case "script":
				srcAttr := ""
				mainAttr := ""
				typeAttr := ""
				for moreAttr {
					var key, val []byte
					key, val, moreAttr = tokenizer.TagAttr()
					if bytes.Equal(key, []byte("src")) {
						srcAttr = string(val)
					} else if bytes.Equal(key, []byte("main")) {
						mainAttr = string(val)
					} else if bytes.Equal(key, []byte("type")) {
						typeAttr = string(val)
					}
				}
				if typeAttr == "importmap" {
					tokenizer.Next()
					innerText := bytes.TrimSpace(tokenizer.Text())
					if len(innerText) > 0 {
						err := json.Unmarshal(innerText, &importMap)
						if err == nil {
							importMap.Src = ctx
						}
					}
				} else if srcAttr == "" {
					// inline script content
					tokenizer.Next()
					input = append(input, string(tokenizer.Text()))
				} else {
					if mainAttr != "" && isHttpSepcifier(srcAttr) {
						if !isHttpSepcifier(mainAttr) && endsWith(mainAttr, esExts...) {
							jsEntries[mainAttr] = struct{}{}
						}
					} else if !isHttpSepcifier(srcAttr) && endsWith(srcAttr, esExts...) {
						jsEntries[srcAttr] = struct{}{}
					}
				}
			case "link", "meta", "title", "base", "head", "noscript", "slot", "template", "option":
				// ignore
			default:
				input = append(input, string(tokenizer.Raw()))
			}
		}
	}
	if configCSS == "" {
		configCSSFilename := filepath.Join(filepath.Dir(imHtmlFilename), "uno.css")
		if _, err := os.Stat(configCSSFilename); err != nil && os.IsNotExist(err) {
			configCSSFilename = filepath.Join(h.rootDir, "uno.css")
		}
		data, err := os.ReadFile(configCSSFilename)
		if err != nil && os.IsExist(err) {
			http.Error(w, "Internal Server Error", 500)
			return
		}
		if len(data) > 0 {
			configCSS = string(data)
		}
	}
	for entry := range jsEntries {
		tree, err := walkDependencyTree(filepath.Join(filepath.Dir(imHtmlFilename), entry), importMap)
		if err == nil {
			for _, code := range tree {
				input = append(input, string(code))
			}
		}
	}
	sha := sha1.New()
	sha.Write([]byte(configCSS))
	for _, s := range input {
		sha.Write([]byte(s))
	}
	etag := fmt.Sprintf("\"%x\"", sha.Sum(nil))
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	cacheKey := fmt.Sprintf("unocss-%s", ctx)
	etagCacheKey := fmt.Sprintf("unocss-%s.etag", ctx)
	if v, ok := h.cache.Load(cacheKey); ok {
		if e, ok := h.cache.Load(etagCacheKey); ok {
			if e.(string) == etag {
				header := w.Header()
				header.Set("Content-Type", "text/css; charset=utf-8")
				header.Set("Cache-Control", "max-age=0, must-revalidate")
				header.Set("Etag", etag)
				w.Write(v.([]byte))
				return
			}
		}
	}
	loader, err := h.getLoader()
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Internal Server Error", 500)
		return
	}
	css, err := loader.Load("unocss", configCSS, strings.Join(input, "\n"))
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Internal Server Error", 500)
		return
	}
	h.cache.Store(cacheKey, []byte(css))
	h.cache.Store(etagCacheKey, etag)
	header := w.Header()
	header.Set("Content-Type", "text/css; charset=utf-8")
	header.Set("Cache-Control", "max-age=0, must-revalidate")
	header.Set("Etag", etag)
	w.Write([]byte(css))
}

func (h *H) ServeInternalJS(w http.ResponseWriter, r *http.Request, name string) {
	data, err := h.assets.ReadFile("assets/" + name + ".js")
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	etag := fmt.Sprintf("w/\"%d\"", VERSION)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "application/javascript; charset=utf-8")
	header.Set("Cache-Control", "max-age=0, must-revalidate")
	header.Set("Etag", etag)
	w.Write(data)
}

func (h *H) ServeHmrWS(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Upgrade") != "websocket" {
		http.Error(w, "Bad Request", 400)
		return
	}
	var upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	defer conn.Close()
	watchList := make(map[string]int64)
	h.rwlock.Lock()
	h.wsConns[conn] = watchList
	h.rwlock.Unlock()
	defer func() {
		h.rwlock.Lock()
		delete(h.wsConns, conn)
		h.rwlock.Unlock()
	}()
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if messageType == websocket.TextMessage {
			msg := string(data)
			if strings.HasPrefix(msg, "watch:") {
				filename := msg[6:]
				if filename != "" {
					filename := utils.CleanPath(filename)
					fi, err := os.Lstat(filepath.Join(h.rootDir, filename))
					if err != nil {
						if os.IsNotExist(err) {
							// file not found, watch if it's created
							watchList[filename] = 0
						} else {
							conn.WriteMessage(websocket.TextMessage, []byte("error:cound not watch "+filename))
						}
					} else {
						watchList[filename] = fi.ModTime().UnixMilli()
					}
				}
			}
		}
	}
}

func (h *H) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pathname := r.URL.Path
	switch pathname {
	case "/@unocss":
		h.ServeUnoCSS(w, r)
		return
	case "/@hmr", "/@refresh", "/@prefresh", "/@vdr":
		h.ServeInternalJS(w, r, pathname[2:])
		return
	case "/@hmr-ws":
		if h.wsConns == nil {
			h.wsConns = make(map[*websocket.Conn]map[string]int64)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					h.rwlock.RLock()
					for conn, watchList := range h.wsConns {
						for filename, mtime := range watchList {
							fi, err := os.Lstat(filepath.Join(h.rootDir, filename))
							if err != nil {
								if os.IsNotExist(err) {
									conn.WriteMessage(websocket.TextMessage, []byte("remove:"+filename))
									watchList[filename] = 0
								}
							} else if modtime := fi.ModTime().UnixMilli(); modtime > mtime {
								kind := "modify"
								if mtime == 0 {
									kind = "create"
								}
								conn.WriteMessage(websocket.TextMessage, []byte(kind+":"+filename))
								watchList[filename] = modtime
							}
						}
					}
					h.rwlock.RUnlock()
				}
			}()
		}
		h.ServeHmrWS(w, r)
		return
	}
	filename := filepath.Join(h.rootDir, pathname)
	fi, err := os.Lstat(filename)
	if err == nil && fi.IsDir() {
		if pathname != "/" && !strings.HasSuffix(pathname, "/") {
			http.Redirect(w, r, pathname+"/", http.StatusMovedPermanently)
			return
		}
		pathname = pathname + "/index.html"
		filename = filepath.Join(h.rootDir, pathname)
		fi, err = os.Lstat(filename)
	}
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", 404)
		} else {
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}
	if fi.IsDir() {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	header := w.Header()
	switch filepath.Ext(filename) {
	case ".html":
		etag := fmt.Sprintf("w/\"%d-%d-%d\"", fi.ModTime().UnixMilli(), fi.Size(), VERSION)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		header.Set("Content-Type", "text/html; charset=utf-8")
		header.Set("Cache-Control", "max-age=0, must-revalidate")
		header.Set("Etag", etag)
		h.ServeHtml(w, r, pathname)
	case ".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx", ".vue", ".svelte":
		h.ServeModule(w, r, pathname)
	default:
		etag := fmt.Sprintf("w/\"%d-%d-%d\"", fi.ModTime().UnixMilli(), fi.Size(), VERSION)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		file, err := os.Open(filename)
		if err != nil {
			http.Error(w, "Internal Server Error", 500)
			return
		}
		defer file.Close()
		mtype := getMIMEType(filename)
		if mtype == "" {
			mtype = "application/octet-stream"
		}
		header.Set("Content-Type", mtype)
		header.Set("Cache-Control", "max-age=0, must-revalidate")
		header.Set("Etag", etag)
		io.Copy(w, file)
	}
}

func (h *H) getLoader() (loader *LoaderWorker, err error) {
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.loader != nil {
		return h.loader, nil
	}
	loaderJs, err := h.assets.ReadFile("assets/loader.js")
	if err != nil {
		return

	}
	loader = &LoaderWorker{}
	err = loader.Start(loaderJs)
	if err != nil {
		return nil, err
	}
	h.loader = loader
	return
}

func Serve(assets *embed.FS, rootDir string, port int) (err error) {
	if rootDir == "" {
		rootDir, err = os.Getwd()
	} else {
		rootDir, err = filepath.Abs(rootDir)
		if err == nil {
			var fi os.FileInfo
			fi, err = os.Stat(rootDir)
			if err == nil && !fi.IsDir() {
				err = fmt.Errorf("stat %s: not a directory", rootDir)
			}
			if err != nil && os.IsExist(err) {
				err = nil
			}
		}
	}
	if err != nil {
		os.Stderr.WriteString(err.Error())
		return err
	}
	server := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: &H{assets: assets, rootDir: rootDir}}
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		os.Stderr.WriteString(err.Error())
		return err
	}
	fmt.Printf("Server is ready on http://localhost:%d\n", port)
	return server.Serve(ln)
}
