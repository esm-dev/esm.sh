package web

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/esm-dev/esm.sh/server/common"
	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/gorilla/websocket"
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

type WebServer struct {
	config           *Config
	loaderWorker     *LoaderWorker
	loaderCache      sync.Map
	watchDataMapLock sync.RWMutex
	watchData        map[*websocket.Conn]map[string]int64
}

func New(config Config) *WebServer {
	if config.AppDir == "" {
		config.AppDir, _ = os.Getwd()
	}
	if config.Fallback == "" {
		config.Fallback = "/index.html"
	}
	s := &WebServer{config: &config}
	go func() {
		err := s.startLoaderWorker()
		if err != nil {
			fmt.Println(term.Red("Failed to start loader worker: " + err.Error()))
		}
	}()
	return s
}

func (s *WebServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
			pathname = s.config.Fallback
			filename = filepath.Join(s.config.AppDir, pathname)
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
		query := r.URL.Query()
		if query.Has("url") {
			header := w.Header()
			header.Set("Content-Type", "application/javascript; charset=utf-8")
			header.Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Write([]byte(`const url = new URL(import.meta.url);url.searchParams.delete("url");export default url.href;`))
			return
		}
		switch extname := filepath.Ext(filename); extname {
		case ".html":
			etag := fmt.Sprintf("w/\"%x-%x-v%d\"", fi.ModTime().UnixMilli(), fi.Size(), VERSION)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			header := w.Header()
			header.Set("Content-Type", "text/html; charset=utf-8")
			header.Set("Cache-Control", "max-age=0, must-revalidate")
			header.Set("Etag", etag)
			s.ServeHtml(w, r, pathname)
		case ".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx", ".vue", ".svelte":
			if !query.Has("raw") {
				s.ServeModule(w, r, pathname, nil)
				return
			}
			fallthrough
		case ".css":
			if query.Has("module") {
				s.ServeCSSModule(w, r, pathname, query)
				return
			}
			if strings.HasSuffix(pathname, "/uno.css") {
				s.ServeUnoCSS(w, r)
				return
			}
			fallthrough
		case ".md":
			if !query.Has("raw") {
				markdown, err := os.ReadFile(filename)
				if err != nil {
					http.Error(w, "Internal Server Error", 500)
					return
				}
				if query.Has("jsx") {
					jsxCode, err := common.RenderMarkdown(markdown, common.MarkdownRenderKindJSX)
					if err != nil {
						http.Error(w, "Failed to render markdown to jsx", 500)
						return
					}
					s.ServeModule(w, r, pathname+"?jsx", jsxCode)
				} else if query.Has("svelte") {
					svelteCode, err := common.RenderMarkdown(markdown, common.MarkdownRenderKindSvelte)
					if err != nil {
						http.Error(w, "Failed to render markdown to svelte component", 500)
						return
					}
					s.ServeModule(w, r, pathname+"?svelte", svelteCode)
				} else if query.Has("vue") {
					vueCode, err := common.RenderMarkdown(markdown, common.MarkdownRenderKindVue)
					if err != nil {
						http.Error(w, "Failed to render markdown to vue component", 500)
						return
					}
					s.ServeModule(w, r, pathname+"?vue", vueCode)
				} else {
					js, err := common.RenderMarkdown(markdown, common.MarkdownRenderKindJS)
					if err != nil {
						http.Error(w, "Failed to render markdown", 500)
						return
					}
					etag := fmt.Sprintf("w/\"%x-%x-v%d\"", fi.ModTime().UnixMilli(), fi.Size(), VERSION)
					if r.Header.Get("If-None-Match") == etag && !query.Has("t") {
						w.WriteHeader(http.StatusNotModified)
						return
					}
					if r.Header.Get("If-None-Match") == etag && !query.Has("t") {
						w.WriteHeader(http.StatusNotModified)
						return
					}
					header := w.Header()
					header.Set("Content-Type", "application/javascript; charset=utf-8")
					if !query.Has("t") {
						header.Set("Cache-Control", "max-age=0, must-revalidate")
						header.Set("Etag", etag)
					}
					w.Write(js)
				}
				return
			}
			fallthrough
		default:
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
			contentType := common.ContentType(filename)
			if contentType == "" {
				contentType = "application/octet-stream"
			}
			header := w.Header()
			header.Set("Content-Type", contentType)
			if !query.Has("t") {
				header.Set("Cache-Control", "max-age=0, must-revalidate")
				header.Set("Etag", etag)
			}
			io.Copy(w, file)
		}
	}
}

func (s *WebServer) ServeHtml(w http.ResponseWriter, r *http.Request, pathname string) {
	htmlFile, err := os.Open(filepath.Join(s.config.AppDir, pathname))
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "Not Found", 404)
		} else {
			http.Error(w, "Internal Server Error", 500)
		}
		return
	}
	defer htmlFile.Close()

	u := url.URL{Path: pathname}
	tokenizer := html.NewTokenizer(htmlFile)
	cssLinks := []string{}
	unocss := ""
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
				srcAttr := attrs["src"]
				hrefAttr := attrs["href"]
				if !isHttpSepcifier(srcAttr) {
					srcAttr, _ = utils.SplitByFirstByte(srcAttr, '?')
					srcAttr = u.ResolveReference(&url.URL{Path: srcAttr}).Path
					w.Write([]byte("<script"))
					for attrKey, attrVal := range attrs {
						switch attrKey {
						case "src":
							w.Write([]byte(" src=\""))
							w.Write([]byte(srcAttr + "?im=" + base64.RawURLEncoding.EncodeToString([]byte(pathname))))
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
				} else if hrefAttr != "" {
					// replace `<script src="https://esm.sh/x" href="..."></script>`
					// with `<script type="module" src="..."></script>`
					if srcUrl, parseErr := url.Parse(srcAttr); parseErr == nil && srcUrl.Path == "/x" {
						hrefAttr, _ = utils.SplitByFirstByte(hrefAttr, '?')
						hrefAttr = u.ResolveReference(&url.URL{Path: hrefAttr}).Path
						if hrefAttr == "uno.css" || strings.HasSuffix(hrefAttr, "/uno.css") {
							if unocss == "" {
								unocssHref := hrefAttr + "?ctx=" + base64.RawURLEncoding.EncodeToString([]byte(pathname))
								w.Write([]byte("<link rel=\"stylesheet\" href=\""))
								w.Write([]byte(unocssHref))
								w.Write([]byte{'"', '>'})
								unocss = unocssHref
							}
							overriding = "script"
						} else {
							w.Write([]byte("<script type=\"module\""))
							for attrKey, attrVal := range attrs {
								switch attrKey {
								case "href", "type":
									// strip
								case "src":
									w.Write([]byte(" src=\""))
									w.Write([]byte(hrefAttr + "?im=" + base64.RawURLEncoding.EncodeToString([]byte(pathname))))
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
						}
						continue
					}
				}
			case "link":
				href := attrs["href"]
				if !isHttpSepcifier(href) {
					href = u.ResolveReference(&url.URL{Path: href}).Path
					if href == "uno.css" || strings.HasSuffix(href, "/uno.css") {
						if unocss == "" {
							href = href + "?ctx=" + base64.RawURLEncoding.EncodeToString([]byte(pathname))
							w.Write([]byte("<link rel=\"stylesheet\" href=\""))
							w.Write([]byte(href))
							w.Write([]byte{'"', '>'})
							unocss = href
						}
						continue
					} else {
						cssLinks = append(cssLinks, href)
					}
					w.Write([]byte("<link"))
					for attrKey, attrVal := range attrs {
						switch attrKey {
						case "href":
							w.Write([]byte(" href=\""))
							w.Write([]byte(href))
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
		fmt.Fprintf(w, `<script type="module">import createHotContext from"/@hmr";const hot=createHotContext("%s");hot.watch(()=>location.reload());`, pathname)
		// reload the page when the css file is modified
		if len(cssLinks) > 0 {
			for _, cssLink := range cssLinks {
				u := &url.URL{Path: pathname}
				u = u.ResolveReference(&url.URL{Path: cssLink})
				fmt.Fprintf(w, `const linkEl=document.querySelector('link[rel="stylesheet"][href="%s"]');hot.watch("%s",(kind,filename)=>{if(kind==="modify")linkEl.href=filename+"?t="+Date.now().toString(36)});`, cssLink, u.Path)
			}
		}
		// reload the unocss when the module dependency tree is changed
		if unocss != "" {
			fmt.Fprintf(w, `hot.watch("*",(kind,filename)=>{if(/\.(js|mjs|jsx|ts|mts|tsx|vue|svelte)$/i.test(filename)){const link=document.head.querySelector("link[rel=stylesheet][href^='%s']");if(link)link.href="%s&t="+Date.now().toString(36)}});`, unocss, unocss)
			u := &url.URL{Path: pathname}
			u = u.ResolveReference(&url.URL{Path: "uno.css"})
			filename := filepath.Join(s.config.AppDir, u.Path)
			if _, err := os.Stat(filename); err != nil && os.IsNotExist(err) {
				u = &url.URL{Path: "/uno.css"}
				filename = filepath.Join(s.config.AppDir, u.Path)
			}
			if _, err := os.Stat(filename); err == nil || os.IsExist(err) {
				// reload the page when the uno.css file is modified
				w.Write([]byte("hot.watch(\""))
				w.Write([]byte(u.Path))
				w.Write([]byte("\",()=>location.reload());"))
			}
		}
		w.Write([]byte("</script>"))
		w.Write([]byte(`<script>console.log("%c💚 Built with esm.sh, please uncheck \"Disable cache\" in Network tab for better DX!", "color:green")</script>`))
	}
}

func (s *WebServer) ServeModule(w http.ResponseWriter, r *http.Request, pathname string, sourceCode []byte) {
	query := r.URL.Query()
	im, err := base64.RawURLEncoding.DecodeString(query.Get("im"))
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	imHtmlFilename := filepath.Join(s.config.AppDir, string(im))
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
	var importMap common.ImportMap
	tokenizer := html.NewTokenizer(imHtmlFile)
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
					if json.Unmarshal(tokenizer.Text(), &importMap) != nil {
						header := w.Header()
						header.Set("Content-Type", "application/javascript; charset=utf-8")
						header.Set("Cache-Control", "max-age=0, must-revalidate")
						w.Write([]byte(`throw new Error("Failed to parse import map: invalid JSON")`))
						return
					}
					importMap.Src = "file://" + string(im)
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
	var modTime uint64
	var size int64
	if sourceCode != nil {
		sha := xxhash.New()
		sha.Write(sourceCode)
		modTime = sha.Sum64()
		size = int64(len(sourceCode))
	} else {
		fi, err := os.Lstat(filepath.Join(s.config.AppDir, pathname))
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
	etag := fmt.Sprintf("w/\"%x-%x-v%d\"", modTime, size, VERSION)
	if r.Header.Get("If-None-Match") == etag && !query.Has("t") {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	cacheKey := fmt.Sprintf("module-%s", pathname)
	etagCacheKey := fmt.Sprintf("module-%s.etag", pathname)
	if js, ok := s.loaderCache.Load(cacheKey); ok {
		if e, ok := s.loaderCache.Load(etagCacheKey); ok {
			if e.(string) == etag {
				header := w.Header()
				header.Set("Content-Type", "application/javascript; charset=utf-8")
				if !query.Has("t") {
					header.Set("Cache-Control", "max-age=0, must-revalidate")
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
	args := []any{pathname, importMap, nil, s.config.Dev}
	if sourceCode != nil {
		args[2] = string(sourceCode)
	}
	_, js, err := s.loaderWorker.Load("module", args)
	if err != nil {
		fmt.Println(term.Red("[error] " + err.Error()))
		http.Error(w, "Internal Server Error", 500)
		return
	}
	s.loaderCache.Store(cacheKey, []byte(js))
	s.loaderCache.Store(etagCacheKey, etag)
	header := w.Header()
	header.Set("Content-Type", "application/javascript; charset=utf-8")
	if !query.Has("t") {
		header.Set("Cache-Control", "max-age=0, must-revalidate")
		header.Set("Etag", etag)
	}
	w.Write([]byte(js))
}

func (s *WebServer) ServeCSSModule(w http.ResponseWriter, r *http.Request, pathname string, query url.Values) {
	ret := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:      []string{filepath.Join(s.config.AppDir, pathname)},
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
	sha := xxhash.New()
	sha.Write(css)
	etag := fmt.Sprintf("w/\"%x-v%d\"", sha.Sum(nil), VERSION)
	if r.Header.Get("If-None-Match") == etag && !query.Has("t") {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	header := w.Header()
	header.Set("Content-Type", "application/javascript; charset=utf-8")
	if !query.Has("t") {
		header.Set("Cache-Control", "max-age=0, must-revalidate")
		header.Set("Etag", etag)
	}
	fmt.Fprintf(w, `const CSS=%s;`, string(utils.MustEncodeJSON(string(css))))
	w.Write([]byte(`let styleEl;`))
	w.Write([]byte(`function applyCSS(css){if(styleEl)styleEl.textContent=css;else{styleEl=document.createElement("style");styleEl.textContent=css;document.head.appendChild(styleEl);}}`))
	if s.config.Dev {
		w.Write([]byte(`!(new URL(import.meta.url)).searchParams.has("t")&&applyCSS(CSS);`))
		w.Write([]byte(`import createHotContext from"/@hmr";`))
		fmt.Fprintf(w, `createHotContext("%s").accept(m=>applyCSS(m.default));`, pathname)
	} else {
		w.Write([]byte(`applyCSS(CSS);`))
	}
	w.Write([]byte(`export default CSS;`))
}

func (s *WebServer) ServeUnoCSS(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	ctx, err := base64.RawURLEncoding.DecodeString(query.Get("ctx"))
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	imHtmlFilename := filepath.Join(s.config.AppDir, string(ctx))
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

	contents := [][]byte{}
	jsEntries := map[string]struct{}{}
	importMap := common.ImportMap{}
	tokenizer := html.NewTokenizer(imHtmlFile)
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
					if bytes.Equal(key, []byte("src")) {
						srcAttr = string(val)
					} else if bytes.Equal(key, []byte("href")) {
						hrefAttr = string(val)
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
							importMap.Src = string(ctx)
						}
					}
				} else if srcAttr == "" {
					// inline script content
					tokenizer.Next()
					contents = append(contents, tokenizer.Text())
				} else {
					if hrefAttr != "" && isHttpSepcifier(srcAttr) {
						if !isHttpSepcifier(hrefAttr) && endsWith(hrefAttr, moduleExts...) {
							jsEntries[hrefAttr] = struct{}{}
						}
					} else if !isHttpSepcifier(srcAttr) && endsWith(srcAttr, moduleExts...) {
						jsEntries[srcAttr] = struct{}{}
					}
				}
			case "link", "meta", "title", "base", "head", "noscript", "slot", "template", "option":
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
		t, err := s.analyzeDependencyTree(filepath.Join(filepath.Dir(imHtmlFilename), entry), importMap)
		if err == nil {
			for filename, code := range t {
				tree[filename] = code
			}
		}
	}
	sha := xxhash.New()
	sha.Write([]byte(configCSS))
	keys := make([]string, 0, len(tree))
	for k := range tree {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		code := tree[k]
		contents = append(contents, code)
		sha.Write(code)
	}
	etag := fmt.Sprintf("w/\"%x-v%d\"", sha.Sum(nil), VERSION)
	if r.Header.Get("If-None-Match") == etag && !query.Has("t") {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	cacheKey := fmt.Sprintf("unocss-%s", ctx)
	etagCacheKey := fmt.Sprintf("unocss-%s.etag", ctx)
	if css, ok := s.loaderCache.Load(cacheKey); ok {
		if e, ok := s.loaderCache.Load(etagCacheKey); ok {
			if e.(string) == etag {
				header := w.Header()
				header.Set("Content-Type", "text/css; charset=utf-8")
				if !query.Has("t") {
					header.Set("Cache-Control", "max-age=0, must-revalidate")
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
	_, css, err := s.loaderWorker.Load("unocss", []any{r.URL.Path + "?" + r.URL.RawQuery, string(bytes.Join(contents, []byte{'\n'})), config})
	if err != nil {
		fmt.Println(term.Red("[error] " + err.Error()))
		http.Error(w, "Internal Server Error", 500)
		return
	}
	s.loaderCache.Store(cacheKey, []byte(css))
	s.loaderCache.Store(etagCacheKey, etag)
	header := w.Header()
	header.Set("Content-Type", "text/css; charset=utf-8")
	if !query.Has("t") {
		header.Set("Cache-Control", "max-age=0, must-revalidate")
		header.Set("Etag", etag)
	}
	w.Write([]byte(css))
}

func (s *WebServer) ServeInternalJS(w http.ResponseWriter, r *http.Request, name string) {
	data, err := efs.ReadFile("internal/" + name + ".js")
	if err != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	sha := xxhash.New()
	sha.Write(data)
	etag := fmt.Sprintf("w/\"%x\"", sha.Sum(nil))
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

func (s *WebServer) ServeHmrWS(w http.ResponseWriter, r *http.Request) {
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

func (s *WebServer) analyzeDependencyTree(entry string, importMap common.ImportMap) (tree map[string][]byte, err error) {
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
						path, _ := importMap.Resolve(args.Path)
						if isHttpSepcifier(path) || (!isRelPathSpecifier(path) && !isAbsPathSpecifier(path)) {
							return esbuild.OnResolveResult{Path: path, External: true}, nil
						}
						if endsWith(path, moduleExts...) {
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
							sha := xxhash.New()
							sha.Write(data)
							etag := hex.EncodeToString(sha.Sum(nil))
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
							lang, code, err := s.loaderWorker.Load(ext[1:], []any{pathname, contents, importMap})
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

func (s *WebServer) watchFS() {
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

func (s *WebServer) purgeLoaderCache(filename string) {
	s.loaderCache.Delete(fmt.Sprintf("module-%s", filename))
	s.loaderCache.Delete(fmt.Sprintf("module-%s.etag", filename))
	if strings.HasSuffix(filename, ".vue") || strings.HasSuffix(filename, ".svelte") {
		s.loaderCache.Delete(fmt.Sprintf("preload-%s", filename))
		s.loaderCache.Delete(fmt.Sprintf("preload-%s.etag", filename))
		s.loaderCache.Delete(fmt.Sprintf("preload-%s.lang", filename))
	}
}

func (s *WebServer) startLoaderWorker() (err error) {
	if s.loaderWorker != nil {
		return nil
	}
	loaderJs, err := efs.ReadFile("internal/loader.js")
	if err != nil {
		return
	}
	worker := &LoaderWorker{}
	err = worker.Start(s.config.AppDir, loaderJs)
	if err != nil {
		return err
	}
	go worker.Load("module", []any{"_.tsx", nil, "", false})
	go func() {
		entries, err := os.ReadDir(s.config.AppDir)
		if err == nil {
			for _, entry := range entries {
				if entry.Type().IsRegular() && entry.Name() == "uno.css" {
					go worker.Load("unocss", []any{"_uno.css", "flex"})
				}
			}
		}
	}()
	s.loaderWorker = worker
	return
}
