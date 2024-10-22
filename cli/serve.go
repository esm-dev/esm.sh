package cli

import (
	"bytes"
	"crypto/sha1"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ije/gox/utils"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type H struct {
	assets  *embed.FS
	rootDir string
	wsConns map[*websocket.Conn]map[string]int64
	lock    sync.RWMutex
	cache   sync.Map
}

type LoaderOutput struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

type ImportMap struct {
	Src     string                       `json:"$src,omitempty"`
	Imports map[string]string            `json:"imports,omitempty"`
	Scopes  map[string]map[string]string `json:"scopes,omitempty"`
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
	skipStyle := false
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if skipStyle {
			if tt == html.EndTagToken && tokenizer.Token().DataAtom == atom.Style {
				skipStyle = false
			}
			continue
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
			} else if token.DataAtom == atom.Link {
				var relAttr string
				var hrefAttr string
				for _, attr := range token.Attr {
					if attr.Key == "rel" {
						relAttr = attr.Val
					} else if attr.Key == "href" {
						hrefAttr = attr.Val
					}
				}
				// replace `<link rel="stylesheet" href="https://esm.sh/uno">`
				// with `<link rel="stylesheet" href="/@unocss">`
				if (strings.HasPrefix(hrefAttr, "http://") || strings.HasPrefix(hrefAttr, "https://")) && strings.HasSuffix(hrefAttr, "/uno") && relAttr == "stylesheet" {
					w.Write([]byte(fmt.Sprintf(`<link rel="stylesheet" href="/@unocss?ctx=%s">`, btoaUrl(pathname))))
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
				// hide `<style type="uno/css">...</style>`
				if typeAttr == "uno/css" {
					// skipStyle = true
					// continue
				}
			}
		}
		w.Write(tokenizer.Raw())
	}
	// reload the page when the html file is modified
	fmt.Fprintf(w, `<script type="module">import createHotContext from"/@hmr";createHotContext("%s").watch(()=>location.reload())</script>`, pathname)
}

func (h *H) ServeTSX(w http.ResponseWriter, r *http.Request, pathname string) {
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
						http.Error(w, "Invalid import map", 400)
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
	cacheKey := fmt.Sprintf("unocss-%s", pathname)
	etagCacheKey := fmt.Sprintf("unocss-%s.cache", pathname)
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
	var output LoaderOutput
	err = h.runjs("tsx-loader", []any{pathname, string(code), importMap}, &output)
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Internal Server Error", 500)
		return
	}
	if output.Error != "" {
		fmt.Println(output.Error)
		http.Error(w, "Internal Server Error", 500)
		return
	}
	js := []byte(output.Code)
	h.cache.Store(cacheKey, js)
	h.cache.Store(etagCacheKey, etag)
	header := w.Header()
	header.Set("Content-Type", "application/javascript; charset=utf-8")
	header.Set("Cache-Control", "max-age=0, must-revalidate")
	header.Set("Etag", etag)
	w.Write(js)
}

func (h *H) ServeVue(w http.ResponseWriter, r *http.Request, pathname string) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("Not Implemented"))
}

func (h *H) ServeSvelte(w http.ResponseWriter, r *http.Request, pathname string) {
	w.WriteHeader(http.StatusNotImplemented)
	w.Write([]byte("Not Implemented"))
}

func (h *H) ServeUnoCSS(w http.ResponseWriter, r *http.Request) {
	ctx, err := atobUrl(r.URL.Query().Get("ctx"))
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	imHtmlFilename := filepath.Join(h.rootDir, string(ctx))
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
	tokenizer := html.NewTokenizer(imHtmlFile)
	inHead := false
	configCSS := ""
	input := []string{}
	scripts := []string{}
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken {
			name, moreAttr := tokenizer.TagName()
			if bytes.Equal(name, []byte("head")) {
				inHead = true
			}
			if inHead {
				if bytes.Equal(name, []byte("style")) {
					for moreAttr {
						var key, val []byte
						key, val, moreAttr = tokenizer.TagAttr()
						if bytes.Equal(key, []byte("type")) && bytes.Equal(val, []byte("uno/css")) {
							tokenizer.Next()
							innerText := bytes.TrimSpace(tokenizer.Text())
							if len(innerText) > 0 {
								configCSS = string(innerText)
							}
							break
						}
					}
				}
			} else if bytes.Equal(name, []byte("script")) {
				srcAttr := ""
				mainAttr := ""
				for moreAttr {
					var key, val []byte
					key, val, moreAttr = tokenizer.TagAttr()
					if bytes.Equal(key, []byte("src")) {
						srcAttr = string(val)
						if mainAttr != "" || !strings.HasSuffix(srcAttr, "/run") {
							break
						}
					} else if bytes.Equal(key, []byte("main")) {
						mainAttr = string(val)
						if srcAttr != "" {
							break
						}
					}
				}
				if srcAttr == "" {
					tokenizer.Next()
					input = append(input, string(tokenizer.Text()))
				} else {
					if mainAttr != "" {
						if !isHttpSepcifier(mainAttr) {
							scripts = append(scripts, mainAttr)
						}
					} else if !isHttpSepcifier(srcAttr) {
						scripts = append(scripts, srcAttr)
					}
				}
			} else {
				for moreAttr {
					var key, val []byte
					key, val, moreAttr = tokenizer.TagAttr()
					if bytes.Equal(key, []byte("class")) {
						input = append(input, "\""+string(val)+"\"")
					} else if !isW3CStandardAttribute(string(key)) {
						input = append(input, string(key)+"=\""+string(val)+"\"")
					}
				}
			}
		} else if tt == html.EndTagToken {
			name, _ := tokenizer.TagName()
			if bytes.Equal(name, []byte("head")) {
				inHead = false
			}
		}
	}
	for _, entry := range scripts {
		code, err := bundleModule(filepath.Join(filepath.Dir(imHtmlFilename), entry))
		if err == nil {
			input = append(input, string(code))
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
	var output LoaderOutput
	err = h.runjs("uno-loader", []any{configCSS, input}, &output)
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Internal Server Error", 500)
		return
	}
	if output.Error != "" {
		fmt.Println(output.Error)
		http.Error(w, "Internal Server Error", 500)
		return
	}
	css := []byte(output.Code)
	h.cache.Store(cacheKey, css)
	h.cache.Store(etagCacheKey, etag)
	header := w.Header()
	header.Set("Content-Type", "text/css; charset=utf-8")
	header.Set("Cache-Control", "max-age=0, must-revalidate")
	header.Set("Etag", etag)
	w.Write(css)
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
	h.lock.Lock()
	h.wsConns[conn] = watchList
	h.lock.Unlock()
	defer func() {
		h.lock.Lock()
		delete(h.wsConns, conn)
		h.lock.Unlock()
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
	case "/@hmr", "/@refresh", "/@prefresh":
		h.ServeInternalJS(w, r, pathname[2:])
		return
	case "/@hmr-ws":
		if h.wsConns == nil {
			h.wsConns = make(map[*websocket.Conn]map[string]int64)
			go func() {
				for {
					time.Sleep(100 * time.Millisecond)
					h.lock.RLock()
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
					h.lock.RUnlock()
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
	switch filepath.Ext(filename) {
	case ".html":
		h.ServeHtml(w, r, pathname)
	case ".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx":
		h.ServeTSX(w, r, pathname)
	case ".vue":
		h.ServeVue(w, r, pathname)
	case ".svelte":
		h.ServeSvelte(w, r, pathname)
	default:
		etag := fmt.Sprintf("w/\"%d%d%d\"", fi.ModTime().Unix(), fi.Size(), VERSION)
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
		header := w.Header()
		if mtype != "" {
			header.Set("Content-Type", mtype)
		}
		header.Set("Cache-Control", "max-age=0, must-revalidate")
		header.Set("Etag", etag)
		io.Copy(w, file)
	}
}

func (h *H) runjs(jsName string, intput any, output any) (err error) {
	start := time.Now()
	debug := os.Getenv("DEBUG") == "1"
	defer func() {
		if err == nil && debug {
			fmt.Println("[debug]", "runjs:"+jsName, time.Since(start))
		}
	}()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	jsCode, err := h.assets.ReadFile("assets/" + jsName + ".js")
	if err != nil {
		err = errors.New("js code not found")
		return
	}
	jsPath := filepath.Join(homeDir, ".esm.sh", "run", fmt.Sprintf("%s-%d.js", jsName, VERSION))
	fi, err := os.Stat(jsPath)
	if (err != nil && os.IsNotExist(err)) || (err == nil && fi.Size() != int64(len(jsCode))) || debug {
		os.MkdirAll(filepath.Dir(jsPath), 0755)
		err = os.WriteFile(jsPath, jsCode, 0644)
		if err != nil {
			return
		}
	}
	denoPath, err := getDenoPath()
	if err != nil {
		err = errors.New("deno not found, please install deno first")
		return
	}
	stdin := bytes.NewBuffer(nil)
	stdout := bytes.NewBuffer(nil)
	stderr := bytes.NewBuffer(nil)
	err = json.NewEncoder(stdin).Encode(intput)
	if err != nil {
		return
	}
	cmd := exec.Command(denoPath, "run", "--no-lock", "-A", jsPath)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if err != nil {
		if stderr.Len() > 0 {
			err = errors.New(stderr.String())
		}
		return
	}
	err = json.NewDecoder(stdout).Decode(output)
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
