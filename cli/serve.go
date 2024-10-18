package cli

import (
	"embed"
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
	"github.com/ije/gox/utils"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type H struct {
	assets  *embed.FS
	rootDir string
	wsConns map[*websocket.Conn]map[string]int64
	lock    sync.RWMutex
}

type ServeFile struct {
	pathname string
	file     *os.File
}

func (h *H) ServeHtml(w http.ResponseWriter, r *http.Request, htmlFile *ServeFile) {
	tokenizer := html.NewTokenizer(htmlFile.file)
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
								w.Write([]byte(fmt.Sprintf(` src="%s"`, mainAttr)))
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
					w.Write([]byte(fmt.Sprintf(`<link rel="stylesheet" href="/@unocss?ctx=%s">`, btoaUrl(htmlFile.pathname))))
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
					skipStyle = true
					continue
				}
			}
		}
		w.Write(tokenizer.Raw())
	}
	fmt.Fprintf(w, `<script type="module">import createHotContext from"/@hmr";createHotContext("%s").watch("%s",()=>location.reload())</script>`, htmlFile.pathname, htmlFile.pathname)
}

func (h *H) ServeUnoCSS(w http.ResponseWriter, r *http.Request) {
	ctx, err := atobUrl(r.URL.Query().Get("ctx"))
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	ctxHtmlFile, err := os.Open(filepath.Join(h.rootDir, string(ctx)))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Bad Request", 400)
		} else {
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}
	defer ctxHtmlFile.Close()
	tokenizer := html.NewTokenizer(ctxHtmlFile)
	configcss := ""
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken {
			token := tokenizer.Token()
			if token.DataAtom == atom.Style {
				var typeAttr string
				for _, attr := range token.Attr {
					if attr.Key == "type" {
						typeAttr = attr.Val
						break
					}
				}
				if typeAttr == "uno/css" {
					tokenizer.Next()
					configcss = string(tokenizer.Raw())
					break
				}
			}
		}
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	fmt.Fprintf(w, "/* %s */\n", configcss)
}

func (h *H) ServeInternalJS(w http.ResponseWriter, r *http.Request, name string) {
	data, err := h.assets.ReadFile("assets/" + name + ".js")
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	header := w.Header()
	etag := fmt.Sprintf("w/\"%d\"", VERSION)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	header.Set("etag", etag)
	header.Set("Content-Type", "application/javascript; charset=utf-8")
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
		filename = filepath.Join(filename, "index.html")
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
	header.Set("etag", etag)
	switch filepath.Ext(filename) {
	case ".html":
		filename, _ = filepath.Rel(h.rootDir, filename)
		h.ServeHtml(w, r, &ServeFile{utils.CleanPath(filename), file})
		return
	}
	io.Copy(w, file)
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
