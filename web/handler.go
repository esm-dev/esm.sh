package web

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/internal/importmap"
	"github.com/esm-dev/esm.sh/internal/mime"
	"github.com/gorilla/websocket"
	esbuild "github.com/ije/esbuild-internal/api"
	"github.com/ije/esbuild-internal/xxhash"
	"github.com/ije/gox/term"
	"github.com/ije/gox/utils"
	"golang.org/x/net/html"
)

type Config struct {
	AppDir   string
	Fallback string
	Dev      bool
}

type Handler struct {
	config           *Config
	etagSuffix       string
	loaderWorker     *JSWorker
	loaderCache      sync.Map
	watchDataMapLock sync.RWMutex
	watchData        map[*websocket.Conn]map[string]int64
}

func NewHandler(config Config) *Handler {
	if config.AppDir == "" {
		config.AppDir, _ = os.Getwd()
	}
	s := &Handler{config: &config}
	s.etagSuffix = "-" + VERSION
	if s.config.Dev {
		s.etagSuffix += "-dev"
	}
	go func() {
		err := s.startLoaderWorker()
		if err != nil {
			fmt.Println(term.Red("Failed to start loader worker: " + err.Error()))
		}
	}()
	return s
}

func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pathname := r.URL.Path
	switch pathname {
	case "/@hmr", "/@refresh", "/@prefresh", "/@vdr":
		s.ServeInternalJS(w, r, pathname[2:])
	case "/@hmr-ws":
		if s.watchData == nil {
			s.watchData = make(map[*websocket.Conn]map[string]int64)
			go s.watchFS()
		}
		s.ServeHmrWS(w, r)
	default:
		filename := filepath.Join(s.config.AppDir, pathname)
		fi, err := os.Lstat(filename)
		if err == nil && fi.IsDir() {
			if pathname != "/" && !strings.HasSuffix(pathname, "/") {
				http.Redirect(w, r, pathname+"/", http.StatusMovedPermanently)
				return
			}
			pathname = strings.TrimSuffix(pathname, "/") + "/index.html"
			filename = filepath.Join(s.config.AppDir, pathname)
			fi, err = os.Lstat(filename)
		}
		if err != nil && os.IsNotExist(err) {
			if s.config.Fallback != "" {
				pathname = "/" + strings.TrimPrefix(s.config.Fallback, "/")
				filename = filepath.Join(s.config.AppDir, pathname)
				fi, err = os.Lstat(filename)
			} else {
				pathname = "/404.html"
				filename = filepath.Join(s.config.AppDir, pathname)
				fi, err = os.Lstat(filename)
				if err != nil && os.IsNotExist(err) {
					pathname = "/index.html"
					filename = filepath.Join(s.config.AppDir, pathname)
					fi, err = os.Lstat(filename)
				}
			}
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
		query := r.URL.Query()
		if query.Has("url") {
			header := w.Header()
			header.Set("Content-Type", "application/javascript; charset=utf-8")
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Write([]byte(`const url = new URL(import.meta.url);url.searchParams.delete("url");export default url.href;`))
			return
		}
		switch filepath.Ext(filename) {
		case ".html":
			s.ServeHtml(w, r, pathname)
			return

		case ".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx", ".vue", ".svelte":
			if !query.Has("raw") {
				s.ServeModule(w, r, pathname, query, nil)
				return
			}

		case ".css":
			if !query.Has("raw") {
				if query.Has("module") {
					s.ServeCSSModule(w, r, query)
					return
				}
				if strings.HasSuffix(pathname, "/tailwind.css") {
					s.ServeFrameworkCSS(w, r, query, "tailwind")
					return
				}
			}
		}

		etag := fmt.Sprintf("w/\"%x-%x\"", fi.ModTime().UnixMilli(), fi.Size())
		if r.Header.Get("If-None-Match") == etag && !query.Has("t") {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		file, err := os.Open(filename)
		if err != nil {
			http.Error(w, "Internal Server Error", 500)
			return
		}
		defer file.Close()
		contentType := mime.GetContentType(filename)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		header := w.Header()
		header.Set("Content-Type", contentType)
		if !query.Has("t") {
			header.Set("Cache-Control", "public, max-age=0, must-revalidate")
			header.Set("Etag", etag)
		}
		io.Copy(w, file)
	}
}

func (s *Handler) ServeHtml(w http.ResponseWriter, r *http.Request, filename string) {
	fi, err := os.Lstat(filepath.Join(s.config.AppDir, filename))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Bad Request", 400)
		} else {
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}

	etag := fmt.Sprintf("w/\"%x-%x%s\"", fi.ModTime().UnixMilli(), fi.Size(), s.etagSuffix)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	header := w.Header()
	header.Set("Content-Type", "text/html; charset=utf-8")
	header.Set("Cache-Control", "public, max-age=0, must-revalidate")
	header.Set("Etag", etag)

	htmlFile, err := os.Open(filepath.Join(s.config.AppDir, filename))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", 404)
		} else {
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}
	defer htmlFile.Close()

	u := url.URL{Path: filename}
	tokenizer := html.NewTokenizer(htmlFile)
	hotLinks := []string{}
	frameworkCSS := ""
	overriding := ""

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if overriding != "" {
			if tt == html.TextToken {
				continue
			}
			if tt == html.EndTagToken {
				tagName, _ := tokenizer.TagName()
				if string(tagName) == overriding {
					overriding = ""
					continue
				}
			}
			overriding = ""
		}
		if tt == html.StartTagToken {
			tagName, moreAttr := tokenizer.TagName()
			attrs := map[string]string{}
			for moreAttr {
				var key, val []byte
				key, val, moreAttr = tokenizer.TagAttr()
				attrs[string(key)] = string(val)
			}
			switch string(tagName) {
			case "script":
				typeAttr := attrs["type"]
				if typeAttr == "importmap" {
					if s.config.Dev {
						w.Write(tokenizer.Raw())
						if tokenizer.Next() == html.TextToken {
							text := tokenizer.Text()
							im, err := importmap.Parse(nil, text)
							if err != nil {
								fmt.Println(term.Yellow("[warn] invalid importmap: " + err.Error()))
								w.Write(text)
							} else {
								// 1. add `dev` query for esm.sh imports in development mode
								var devImports [][2]string
								im.Imports.Range(func(specifier, modUrl string) bool {
									if isHttpSepcifier(modUrl) {
										imp, err := importmap.ParseEsmPath(modUrl)
										if err == nil {
											u, _ := url.Parse(modUrl)
											switch imp.Name {
											case "react", "react-dom", "preact", "solid-js", "vue":
												if strings.HasSuffix(u.Path, ".mjs") {
													u.Path = u.Path[0:len(u.Path)-4] + ".development.mjs"
												} else if strings.HasSuffix(specifier, "/") {
													if imp.Version == "" {
														return true
													}
													// esm.sh specified route: "https://esm.sh/react@19.2.0&dev/"
													u.Path = strings.Replace(u.Path, "@"+imp.Version, "@"+imp.Version+"&dev", 1)
												} else {
													q := u.Query()
													q.Set("dev", "TRUE")
													u.RawQuery = strings.Replace(q.Encode(), "dev=TRUE", "dev", 1)
												}
												devImports = append(devImports, [2]string{specifier, encodeUrl(u)})
											}
										}
									}
									return true
								})

								// 2. ensure `jsx-dev-runtime` is included in the import map
								var jsxRuntime string
								var jsxRuntimeUrl string
								for _, pkg := range []string{"react", "preact", "solid-js", "mono-jsx/dom", "mono-jsx", "vue"} {
									url, ok := im.Resolve(pkg+"/jsx-runtime", nil)
									if ok {
										jsxRuntime = pkg
										jsxRuntimeUrl = url
										break
									}
								}
								if jsxRuntimeUrl != "" && isHttpSepcifier(jsxRuntimeUrl) && !im.Imports.Has(jsxRuntime+"/jsx-dev-runtime") {
									u, err := url.Parse(jsxRuntimeUrl)
									if err == nil {
										if strings.HasSuffix(u.Path, "/jsx-runtime.mjs") {
											u.Path = u.Path[0:len(u.Path)-16] + "/jsx-dev-runtime.development.mjs"
										} else {
											u.Path = strings.Replace(u.Path, "/jsx-runtime", "/jsx-dev-runtime", 1)
										}
										devImports = append(devImports, [2]string{jsxRuntime + "/jsx-dev-runtime", encodeUrl(u)})
									}
								}

								// 3. override imports
								for _, imp := range devImports {
									im.Imports.Set(imp[0], imp[1])
								}

								// 4. send the modified importmap to client
								w.Write([]byte{'\n'})
								w.Write([]byte(im.FormatJSON(1)))
								w.Write([]byte{'\n', ' ', ' '})
							}
						} else {
							w.Write(tokenizer.Raw())
						}
						continue
					}
				}
			case "link":
				relAttr := attrs["rel"]
				hrefAtr := attrs["href"]
				if !isHttpSepcifier(hrefAtr) {
					href := u.ResolveReference(&url.URL{Path: hrefAtr}).Path
					if href == "/tailwind.css" || strings.HasPrefix(href, "/tailwind.css") || href == "/uno.css" || strings.HasPrefix(href, "/uno.css") {
						if frameworkCSS == "" {
							w.Write([]byte("<link rel=\"stylesheet\" href=\""))
							w.Write([]byte(href))
							w.Write([]byte{'"', '>'})
							frameworkCSS = href
						}
						continue
					} else if relAttr == "stylesheet" || relAttr == "icon" {
						hotLinks = append(hotLinks, hrefAtr)
					}
					w.Write([]byte("<link"))
					for attrKey, attrVal := range attrs {
						switch attrKey {
						case "href":
							w.Write([]byte(" href=\""))
							w.Write([]byte(hrefAtr))
							w.Write([]byte{'"'})
						default:
							w.Write([]byte{' '})
							w.Write([]byte(attrKey))
							if attrVal != "" {
								w.Write([]byte{'=', '"'})
								w.Write([]byte(attrVal))
								w.Write([]byte{'"'})
							}
						}
					}
					w.Write([]byte{'>'})
					continue
				}
			}
		}
		w.Write(tokenizer.Raw())
	}

	if s.config.Dev {
		// reload the page when the html file is modified
		w.Write([]byte(`<script type="module">import createHotContext from"/@hmr";const hot=createHotContext("`))
		w.Write([]byte(filename))
		w.Write([]byte(`"),$=p=>document.querySelector(p);hot.watch(()=>location.reload());`))
		// reload icon/style links when the file is modified
		if len(hotLinks) > 0 {
			w.Write([]byte("for(const href of ["))
			for i, href := range hotLinks {
				if i > 0 {
					w.Write([]byte(","))
				}
				w.Write([]byte{'"'})
				w.Write([]byte(href))
				w.Write([]byte{'"'})
			}
			w.Write([]byte(`]){const el=$("link[href='"+href+"']");hot.watch(href,kind=>{if(kind==="modify")el.href=href+"?t="+Date.now().toString(36)})}`))
		}

		// reload the tailwindcss when the module dependency tree is changed
		if frameworkCSS != "" {
			w.Write([]byte(`{const href="` + frameworkCSS + `",link=$("link[href='"+href+"']");`))
			w.Write([]byte(`hot.watch("*",(kind,filename)=>{if(/\.(js|mjs|jsx|ts|mts|tsx|vue|svelte)$/i.test(filename)){link.href=href+"?t="+Date.now().toString(36)}});`))
			// reload the page when the uno.css file is modified
			w.Write([]byte(`hot.watch("`))
			w.Write([]byte(frameworkCSS))
			w.Write([]byte(`",()=>location.reload())}`))
		}

		w.Write([]byte("</script>"))
		w.Write([]byte(`<script>console.log("%c💚 Built with esm.sh, please uncheck \"Disable cache\" in Network tab for better DX!", "color:green")</script>`))
	}
}

func (s *Handler) ServeModule(w http.ResponseWriter, r *http.Request, filename string, query url.Values, preTransformContent []byte) {
	indexHtmlStat, err := os.Lstat(filepath.Join(s.config.AppDir, "index.html"))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Bad Request", 400)
		} else {
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}

	var modTime uint64
	var size int64
	if preTransformContent != nil {
		xx := xxhash.New()
		xx.Write(preTransformContent)
		modTime = xx.Sum64()
		size = int64(len(preTransformContent))
	} else {
		fi, err := os.Lstat(filepath.Join(s.config.AppDir, filename))
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, "Not Found", 404)
			} else {
				http.Error(w, "Internal Server Error", 500)
			}
			return
		}
		modTime = uint64(fi.ModTime().UnixMilli())
		size = fi.Size()
	}
	etag := fmt.Sprintf("w/\"%x-%x-%x-%x%s\"", modTime, size, indexHtmlStat.ModTime().UnixMilli(), indexHtmlStat.Size(), s.etagSuffix)
	if r.Header.Get("If-None-Match") == etag && !query.Has("t") {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	cacheKey := "module-" + filename
	etagCacheKey := cacheKey + ".etag"
	if js, ok := s.loaderCache.Load(cacheKey); ok {
		if e, ok := s.loaderCache.Load(etagCacheKey); ok {
			if e.(string) == etag {
				header := w.Header()
				header.Set("Content-Type", "application/javascript; charset=utf-8")
				if !query.Has("t") {
					header.Set("Cache-Control", "public, max-age=0, must-revalidate")
					header.Set("Etag", etag)
				}
				w.Write(js.([]byte))
				return
			}
		}
	}
	if s.loaderWorker == nil {
		http.Error(w, "Loader worker not started", 500)
		return
	}
	_, importMap, err := s.getAppImportMap()
	if err != nil {
		http.Error(w, "could not parse import map: "+err.Error(), 500)
		return
	}
	options := map[string]any{"isDev": s.config.Dev}
	args := []any{"tsx", filename, nil, options}
	if strings.HasSuffix(filename, ".vue") || strings.HasSuffix(filename, ".md?vue") {
		if importMap != nil {
			url, ok := importMap.Imports.Get("vue")
			if ok {
				im, err := importmap.ParseEsmPath(url)
				if err != nil {
					http.Error(w, "importmap: failed to parse vue import url: "+url, 500)
					return
				}
				options["vueVersion"] = im.Version
			}
			// if use jsx in a vue app
			if importMap.Imports.Has("vue/jsx-runtime") || importMap.Imports.Has("vue/") {
				options["jsxImportSource"] = "vue"
			}
		}
	} else if strings.HasSuffix(filename, ".svelte") || strings.HasSuffix(filename, ".md?svelte") {
		if importMap != nil {
			url, ok := importMap.Imports.Get("svelte")
			if ok {
				im, err := importmap.ParseEsmPath(url)
				if err != nil {
					http.Error(w, "importmap: failed to parse svelte import url: "+url, 500)
					return
				}
				options["svelteVersion"] = im.Version
			}
		}
	} else {
		if importMap != nil {
			for _, pkg := range []string{"react", "preact", "solid-js", "mono-jsx/dom", "mono-jsx"} {
				if importMap.Imports.Has(pkg+"/jsx-runtime") || importMap.Imports.Has(pkg+"/") {
					options["jsxImportSource"] = pkg
					break
				}
			}
			for _, pkg := range []string{"react", "preact"} {
				if importMap.Imports.Has(pkg) {
					options[pkg] = true
				}
			}
		}
	}
	if preTransformContent != nil {
		args[2] = string(preTransformContent)
	}
	_, js, err := s.loaderWorker.Call(args...)
	if err != nil {
		fmt.Println(term.Red("[error] loader(tsx) " + err.Error()))
		http.Error(w, "Internal Server Error", 500)
		return
	}
	s.loaderCache.Store(cacheKey, []byte(js))
	s.loaderCache.Store(etagCacheKey, etag)
	header := w.Header()
	header.Set("Content-Type", "application/javascript; charset=utf-8")
	if !query.Has("t") {
		header.Set("Cache-Control", "public, max-age=0, must-revalidate")
		header.Set("Etag", etag)
	}
	w.Write([]byte(js))
}

func (s *Handler) ServeCSSModule(w http.ResponseWriter, r *http.Request, query url.Values) {
	ret := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:      []string{filepath.Join(s.config.AppDir, r.URL.Path)},
		Write:            false,
		MinifyWhitespace: true,
		MinifySyntax:     true,
		Target:           esbuild.ES2022,
		Bundle:           true,
	})
	if len(ret.Errors) > 0 {
		fmt.Println(term.Red(ret.Errors[0].Text))
		http.Error(w, "Internal Server Error", 500)
		return
	}
	css := bytes.TrimSpace(ret.OutputFiles[0].Contents)
	xx := xxhash.New()
	xx.Write(css)
	etag := fmt.Sprintf("w/\"%x%s\"", xx.Sum(nil), s.etagSuffix)
	if r.Header.Get("If-None-Match") == etag && !query.Has("t") {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "application/javascript; charset=utf-8")
	if !query.Has("t") {
		header.Set("Cache-Control", "public, max-age=0, must-revalidate")
		header.Set("Etag", etag)
	}
	w.Write([]byte("const css=\""))
	w.Write(bytes.ReplaceAll(css, []byte{'"'}, []byte{'\\', '"'}))
	w.Write([]byte("\";let style,"))
	w.Write([]byte(`applyCSS=css=>{(style??(style=document.head.appendChild(document.createElement("style")))).textContent=css};`))
	if s.config.Dev {
		w.Write([]byte(`import createHot from"/@hmr";`))
		w.Write([]byte("const hot=createHot(import.meta.url);hot.accept(_=>applyCSS(_.default));!hot.locked&&applyCSS(css);"))
	} else {
		w.Write([]byte("applyCSS(css);"))
	}
	w.Write([]byte("export default css;"))
}

func (s *Handler) ServeFrameworkCSS(w http.ResponseWriter, r *http.Request, query url.Values, framework string) {
	indexHtmlFile, err := os.Open(filepath.Join(s.config.AppDir, "index.html"))
	if err != nil {
		fmt.Println(term.Red("[error] failed to open index.html: " + err.Error()))
		http.Error(w, "Internal Server Error", 500)
		return
	}
	defer indexHtmlFile.Close()

	var importMap *importmap.ImportMap

	contents := [][]byte{}
	jsEntries := map[string]struct{}{}
	tokenizer := html.NewTokenizer(indexHtmlFile)
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken {
			name, moreAttr := tokenizer.TagName()
			switch string(name) {
			case "script":
				var (
					typeAttr string
					srcAttr  string
					hrefAttr string
				)
				for moreAttr {
					var key, val []byte
					key, val, moreAttr = tokenizer.TagAttr()
					if bytes.Equal(key, []byte("type")) {
						typeAttr = string(val)
					} else if bytes.Equal(key, []byte("src")) {
						srcAttr = string(val)
					} else if bytes.Equal(key, []byte("href")) {
						hrefAttr = string(val)
					}
				}
				if typeAttr == "importmap" {
					tokenizer.Next()
					innerText := bytes.TrimSpace(tokenizer.Text())
					if len(innerText) > 0 {
						importMap, _ = importmap.Parse(nil, innerText)
					}
				} else if srcAttr == "" {
					// inline script content
					tokenizer.Next()
					contents = append(contents, tokenizer.Text())
				} else {
					if hrefAttr != "" && isHttpSepcifier(srcAttr) {
						if !isHttpSepcifier(hrefAttr) && isModulePath(hrefAttr) {
							jsEntries[hrefAttr] = struct{}{}
						}
					} else if !isHttpSepcifier(srcAttr) && isModulePath(srcAttr) {
						jsEntries[srcAttr] = struct{}{}
					}
				}
			case "link", "meta", "title", "base", "head", "noscript":
				// ignore
			default:
				contents = append(contents, tokenizer.Raw())
			}
		}
	}

	configCSS, err := os.ReadFile(filepath.Join(s.config.AppDir, r.URL.Path))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", 404)
		}
		http.Error(w, "Internal Server Error", 500)
		return
	}

	tree := map[string][]byte{}
	for entry := range jsEntries {
		t, err := s.analyzeDependencyTree(filepath.Join(s.config.AppDir, entry), importMap)
		if err == nil {
			maps.Copy(tree, t)
		}
	}
	xx := xxhash.New()
	xx.Write([]byte(configCSS))
	keys := make([]string, 0, len(tree))
	for k := range tree {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		code := tree[k]
		contents = append(contents, code)
		xx.Write(code)
	}
	etag := fmt.Sprintf("w/\"%x%s\"", xx.Sum(nil), s.etagSuffix)
	if r.Header.Get("If-None-Match") == etag && !query.Has("t") {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	cacheKey := framework
	etagCacheKey := framework + ".etag"
	if css, ok := s.loaderCache.Load(cacheKey); ok {
		if e, ok := s.loaderCache.Load(etagCacheKey); ok {
			if e.(string) == etag {
				header := w.Header()
				header.Set("Content-Type", "text/css; charset=utf-8")
				if !query.Has("t") {
					header.Set("Cache-Control", "public, max-age=0, must-revalidate")
					header.Set("Etag", etag)
				}
				w.Write(css.([]byte))
				return
			}
		}
	}
	if s.loaderWorker == nil {
		http.Error(w, "Loader worker not started", 500)
		return
	}
	config := map[string]any{"filename": r.URL.Path, "css": string(configCSS)}
	_, css, err := s.loaderWorker.Call(framework, r.URL.Path+"?"+r.URL.RawQuery, string(bytes.Join(contents, []byte{'\n'})), config)
	if err != nil {
		fmt.Println(term.Red("[error] loader(" + framework + "): " + err.Error()))
		http.Error(w, "Internal Server Error", 500)
		return
	}
	s.loaderCache.Store(cacheKey, []byte(css))
	s.loaderCache.Store(etagCacheKey, etag)
	header := w.Header()
	header.Set("Content-Type", "text/css; charset=utf-8")
	if !query.Has("t") {
		header.Set("Cache-Control", "public, max-age=0, must-revalidate")
		header.Set("Etag", etag)
	}
	w.Write([]byte(css))
}

func (s *Handler) ServeInternalJS(w http.ResponseWriter, r *http.Request, name string) {
	data, err := efs.ReadFile("internal/" + name + ".js")
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	xx := xxhash.New()
	xx.Write(data)
	etag := fmt.Sprintf("w/\"%x\"", xx.Sum(nil))
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "application/javascript; charset=utf-8")
	header.Set("Cache-Control", "public, max-age=0, must-revalidate")
	header.Set("Etag", etag)
	w.Write(data)
}

func (s *Handler) ServeHmrWS(w http.ResponseWriter, r *http.Request) {
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
	s.watchDataMapLock.Lock()
	s.watchData[conn] = watchList
	s.watchDataMapLock.Unlock()
	defer func() {
		s.watchDataMapLock.Lock()
		delete(s.watchData, conn)
		s.watchDataMapLock.Unlock()
	}()
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if messageType == websocket.TextMessage {
			msg := string(data)
			if strings.HasPrefix(msg, "watch:") {
				pathname := msg[6:]
				if pathname != "" {
					filename, _ := utils.SplitByFirstByte(pathname, '?')
					fi, err := os.Lstat(filepath.Join(s.config.AppDir, filename))
					if err != nil {
						if os.IsNotExist(err) {
							// file not found, watch if it's created
							watchList[pathname] = 0
						} else {
							conn.WriteMessage(websocket.TextMessage, []byte("error:could not watch "+pathname))
						}
					} else {
						watchList[pathname] = fi.ModTime().UnixMilli()
					}
				}
			}
		}
	}
}

func (s *Handler) getAppImportMap() (importMapRaw []byte, importMap *importmap.ImportMap, err error) {
	file, err := os.Open(filepath.Join(s.config.AppDir, "index.html"))
	if err != nil {
		return
	}
	defer file.Close()

	tokenizer := html.NewTokenizer(file)
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken {
			tagName, moreAttr := tokenizer.TagName()
			if string(tagName) == "script" {
				var typeAttr string
				for moreAttr {
					var key, val []byte
					key, val, moreAttr = tokenizer.TagAttr()
					if string(key) == "type" {
						typeAttr = string(val)
						break
					}
				}
				if typeAttr == "importmap" {
					tokenizer.Next()
					importMapRaw = tokenizer.Text()
					importMap, err = importmap.Parse(nil, importMapRaw)
					if err != nil {
						err = errors.New("invalid import map")
						return
					}
					// todo: cache parsed import map
					break
				}
			} else if string(tagName) == "body" {
				break
			}
		} else if tt == html.EndTagToken {
			tagName, _ := tokenizer.TagName()
			if bytes.Equal(tagName, []byte("head")) {
				break
			}
		}
	}
	return
}

func (s *Handler) analyzeDependencyTree(entry string, importMap *importmap.ImportMap) (tree map[string][]byte, err error) {
	tree = make(map[string][]byte)
	ret := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:      []string{entry},
		Target:           esbuild.ES2022,
		Format:           esbuild.FormatESModule,
		Platform:         esbuild.PlatformBrowser,
		JSX:              esbuild.JSXPreserve,
		MinifyWhitespace: true,
		Bundle:           true,
		Write:            false,
		Outdir:           "/esbuild",
		Plugins: []esbuild.Plugin{
			{
				Name: "loader",
				Setup: func(build esbuild.PluginBuild) {
					build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
						url, _ := url.Parse(args.Importer)
						path := args.Path
						if importMap != nil {
							path, _ = importMap.Resolve(args.Path, url)
						}
						if isHttpSepcifier(path) || (!isRelPathSpecifier(path) && !isAbsPathSpecifier(path)) {
							return esbuild.OnResolveResult{Path: path, External: true}, nil
						}
						if isModulePath(path) {
							if isRelPathSpecifier(path) {
								path = filepath.Join(filepath.Dir(args.Importer), path)
							}
							return esbuild.OnResolveResult{Path: path, Namespace: "module", PluginData: path}, nil
						}
						return esbuild.OnResolveResult{}, nil
					})
					build.OnLoad(esbuild.OnLoadOptions{Filter: ".*", Namespace: "module"}, func(args esbuild.OnLoadArgs) (esbuild.OnLoadResult, error) {
						data, err := os.ReadFile(args.Path)
						if err != nil {
							return esbuild.OnLoadResult{}, err
						}
						tree[args.Path] = data
						contents := string(data)
						loader := esbuild.LoaderJS
						pathname := args.PluginData.(string)
						ext := filepath.Ext(pathname)
						switch ext {
						case ".jsx":
							loader = esbuild.LoaderJSX
						case ".ts", ".mts":
							loader = esbuild.LoaderTS
						case ".tsx":
							loader = esbuild.LoaderTSX
						case ".vue", ".svelte":
							xx := xxhash.New()
							xx.Write(data)
							etag := hex.EncodeToString(xx.Sum(nil))
							cacheKey := fmt.Sprintf("preload-%s", pathname)
							etagCacheKey := fmt.Sprintf("preload-%s.etag", pathname)
							langCacheKey := fmt.Sprintf("preload-%s.lang", pathname)
							if js, ok := s.loaderCache.Load(cacheKey); ok {
								if e, ok := s.loaderCache.Load(etagCacheKey); ok {
									if e.(string) == etag {
										contents = string(js.([]byte))
										if lang, ok := s.loaderCache.Load(langCacheKey); ok {
											if lang.(string) == "ts" {
												loader = esbuild.LoaderTS
											}
										}
										break
									}
								}
							}
							if s.loaderWorker == nil {
								return esbuild.OnLoadResult{}, errors.New("loader worker not started")
							}
							framework := ext[1:]
							options := map[string]any{}
							if importMap != nil {
								url, ok := importMap.Imports.Get(framework)
								if ok {
									im, err := importmap.ParseEsmPath(url)
									if err != nil {
										return esbuild.OnLoadResult{}, errors.New("failed to parse " + framework + " import url: " + url)
									}
									options[framework+"Version"] = im.Version
								}
							}
							lang, code, err := s.loaderWorker.Call(framework, pathname, contents, options)
							if err != nil {
								return esbuild.OnLoadResult{}, err
							}
							s.loaderCache.Store(cacheKey, []byte(code))
							s.loaderCache.Store(etagCacheKey, etag)
							s.loaderCache.Store(langCacheKey, lang)
							contents = code
							if lang == "ts" {
								loader = esbuild.LoaderTS
							}
						}
						return esbuild.OnLoadResult{Contents: &contents, Loader: loader}, nil
					})
				},
			},
		},
	})
	if len(ret.Errors) > 0 {
		tree = nil
		err = errors.New(ret.Errors[0].Text)
	}
	return
}

func (s *Handler) watchFS() {
	for {
		time.Sleep(100 * time.Millisecond)
		s.watchDataMapLock.RLock()
		for conn, watchList := range s.watchData {
			for pathname, mtime := range watchList {
				filename, _ := utils.SplitByFirstByte(pathname, '?')
				fi, err := os.Lstat(filepath.Join(s.config.AppDir, filename))
				if err != nil {
					if os.IsNotExist(err) {
						if watchList[pathname] > 0 {
							watchList[pathname] = 0
							s.purgeLoaderCache(pathname)
							conn.WriteMessage(websocket.TextMessage, []byte("remove:"+pathname))
						}
					} else {
						fmt.Println(term.Red("watch: " + err.Error()))
					}
				} else if modtime := fi.ModTime().UnixMilli(); modtime > mtime {
					kind := "modify"
					if mtime == 0 {
						kind = "create"
					}
					watchList[pathname] = modtime
					conn.WriteMessage(websocket.TextMessage, []byte(kind+":"+pathname))
				}
			}
		}
		s.watchDataMapLock.RUnlock()
	}
}

func (s *Handler) purgeLoaderCache(filename string) {
	s.loaderCache.Delete(fmt.Sprintf("module-%s", filename))
	s.loaderCache.Delete(fmt.Sprintf("module-%s.etag", filename))
	if strings.HasSuffix(filename, ".vue") || strings.HasSuffix(filename, ".svelte") {
		s.loaderCache.Delete(fmt.Sprintf("preload-%s", filename))
		s.loaderCache.Delete(fmt.Sprintf("preload-%s.etag", filename))
		s.loaderCache.Delete(fmt.Sprintf("preload-%s.lang", filename))
	}
}

func (s *Handler) startLoaderWorker() (err error) {
	if s.loaderWorker != nil {
		return nil
	}
	loaderWorker := &JSWorker{
		wd:     s.config.AppDir,
		script: "loader.js",
	}
	err = loaderWorker.Start()
	if err != nil {
		return err
	}
	go s.preload()
	s.loaderWorker = loaderWorker
	return
}

func (s *Handler) preload() {
	indexHtmlFilename := filepath.Join(s.config.AppDir, "index.html")
	indexHtmlFile, err := os.Open(indexHtmlFilename)
	if err != nil {
		return
	}
	defer indexHtmlFile.Close()
	entries := map[string]struct{}{}
	tokenizer := html.NewTokenizer(indexHtmlFile)
	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.StartTagToken {
			tagName, moreAttr := tokenizer.TagName()
			if string(tagName) == "script" {
				var (
					srcAttr  string
					hrefAttr string
				)
				for moreAttr {
					var key, val []byte
					key, val, moreAttr = tokenizer.TagAttr()
					if bytes.Equal(key, []byte("src")) {
						srcAttr = string(val)
					} else if bytes.Equal(key, []byte("href")) {
						hrefAttr = string(val)
					}
				}
				if hrefAttr != "" && isHttpSepcifier(srcAttr) {
					if !isHttpSepcifier(hrefAttr) && (hrefAttr == "uno.css" || strings.HasSuffix(hrefAttr, "/uno.css") || isModulePath(hrefAttr)) {
						entries[hrefAttr] = struct{}{}
					}
				} else if !isHttpSepcifier(srcAttr) && isModulePath(srcAttr) {
					entries[srcAttr] = struct{}{}
				}
			}
		}
	}
	if len(entries) > 0 {
		rootUrl := url.URL{Path: "/"}
		w := &dummyResponseWriter{}
		q := url.Values{}
		for entry := range entries {
			pathname := rootUrl.ResolveReference(&url.URL{Path: entry}).Path
			r := &http.Request{
				Method: "GET",
				URL: &url.URL{
					Path: pathname,
				},
			}
			if strings.HasSuffix(entry, "tailwind.css") {
				s.ServeFrameworkCSS(w, r, q, "tailwind")
			} else {
				s.ServeModule(w, r, pathname, q, nil)
			}
		}
	}
}
