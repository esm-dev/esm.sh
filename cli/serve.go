package cli

import (
	"bytes"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

func Serve(efs *embed.FS) (err error) {
	port := flag.Int("port", 3000, "port to serve on")
	rootDir, _ := parseCommandFlag()

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
		}
	}
	if err != nil {
		os.Stderr.WriteString(term.Red(err.Error()))
		return err
	}

	serv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: &DevServer{efs: efs, rootDir: rootDir},
	}
	ln, err := net.Listen("tcp", serv.Addr)
	if err != nil {
		os.Stderr.WriteString(term.Red(err.Error()))
		return err
	}
	fmt.Printf(term.Green("Server is ready on http://localhost:%d\n"), *port)
	return serv.Serve(ln)
}

type DevServer struct {
	efs       *embed.FS
	loader    *LoaderWorker
	rootDir   string
	watchData map[*websocket.Conn]map[string]int64
	rwlock    sync.RWMutex
	lock      sync.RWMutex
	loadCache sync.Map
}

func (d *DevServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pathname := r.URL.Path
	switch pathname {
	case "/@hmr", "/@refresh", "/@prefresh", "/@vdr":
		d.ServeInternalJS(w, r, pathname[2:])
	case "/@uno.css":
		d.ServeUnoCSS(w, r)
	case "/@hmr-ws":
		if d.watchData == nil {
			d.watchData = make(map[*websocket.Conn]map[string]int64)
			go d.watchFS()
		}
		d.ServeHmrWS(w, r)
	default:
		filename := filepath.Join(d.rootDir, pathname)
		fi, err := os.Lstat(filename)
		if err == nil && fi.IsDir() {
			if pathname != "/" && !strings.HasSuffix(pathname, "/") {
				http.Redirect(w, r, pathname+"/", http.StatusMovedPermanently)
				return
			}
			pathname = strings.TrimSuffix(pathname, "/") + "/index.html"
			filename = filepath.Join(d.rootDir, pathname)
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
			etag := fmt.Sprintf("w/\"%d-%d-%d\"", fi.ModTime().UnixMilli(), fi.Size(), VERSION)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			header := w.Header()
			header.Set("Content-Type", "text/html; charset=utf-8")
			header.Set("Cache-Control", "max-age=0, must-revalidate")
			header.Set("Etag", etag)
			d.ServeHtml(w, r, pathname)
		case ".js", ".mjs", ".jsx", ".ts", ".mts", ".tsx", ".vue", ".svelte":
			if !query.Has("raw") {
				d.ServeModule(w, r, pathname, nil)
				return
			}
			fallthrough
		default:
			if extname == ".css" {
				if query.Has("module") {
					d.ServeCSSModule(w, r, pathname, query)
					return
				}
			}
			if extname == ".md" {
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
						d.ServeModule(w, r, pathname+"?jsx", jsxCode)
					} else if query.Has("svelte") {
						svelteCode, err := common.RenderMarkdown(markdown, common.MarkdownRenderKindSvelte)
						if err != nil {
							http.Error(w, "Failed to render markdown to svelte component", 500)
							return
						}
						d.ServeModule(w, r, pathname+"?svelte", svelteCode)
					} else if query.Has("vue") {
						vueCode, err := common.RenderMarkdown(markdown, common.MarkdownRenderKindVue)
						if err != nil {
							http.Error(w, "Failed to render markdown to vue component", 500)
							return
						}
						d.ServeModule(w, r, pathname+"?vue", vueCode)
					} else {
						js, err := common.RenderMarkdown(markdown, common.MarkdownRenderKindJS)
						if err != nil {
							http.Error(w, "Failed to render markdown", 500)
							return
						}
						etag := fmt.Sprintf("w/\"%d-%d-%d\"", fi.ModTime().UnixMilli(), fi.Size(), VERSION)
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
			}
			etag := fmt.Sprintf("w/\"%d-%d-%d\"", fi.ModTime().UnixMilli(), fi.Size(), VERSION)
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

func (d *DevServer) ServeHtml(w http.ResponseWriter, r *http.Request, pathname string) {
	htmlFile, err := os.Open(filepath.Join(d.rootDir, pathname))
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
	hasInlineUnoConfigCSS := false
	unocss := ""
	cssLinks := []string{}

	for {
		tt := tokenizer.Next()
		if tt == html.ErrorToken {
			break
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
				mainAttr := attrs["main"]
				// add `im` query to the main script
				if isRelPathSpecifier(srcAttr) || strings.HasPrefix(srcAttr, "/") {
					w.Write([]byte("<script"))
					for attrKey, attrVal := range attrs {
						if attrKey == "src" {
							srcAttr, _ = utils.SplitByFirstByte(srcAttr, '?')
							w.Write([]byte(fmt.Sprintf(` src="%s?im=%s"`, srcAttr, btoaUrl(pathname))))
						} else {
							if attrVal == "" {
								w.Write([]byte(fmt.Sprintf(` %s`, attrKey)))
							} else {
								w.Write([]byte(fmt.Sprintf(` %s="%s"`, attrKey, attrVal)))
							}
						}
					}
					w.Write([]byte(">"))
					continue
				}
				// replace `<script type="module" src="https://esm.sh/x" main="$main"></script>`
				// with `<script type="module" src="$main"></script>`
				if (strings.HasPrefix(srcAttr, "http://") || strings.HasPrefix(srcAttr, "https://")) && strings.HasSuffix(srcAttr, "/x") && mainAttr != "" {
					w.Write([]byte("<script"))
					for attrKey, attrVal := range attrs {
						if attrKey != "main" {
							if attrKey == "src" {
								mainAttr, _ = utils.SplitByFirstByte(mainAttr, '?')
								w.Write([]byte(fmt.Sprintf(` src="%s"`, mainAttr+"?im="+btoaUrl(pathname))))
							} else {
								if attrVal == "" {
									w.Write([]byte(fmt.Sprintf(` %s`, attrKey)))
								} else {
									w.Write([]byte(fmt.Sprintf(` %s="%s"`, attrKey, attrVal)))
								}
							}
						}
					}
					w.Write([]byte(">"))
					continue
				}
				// replace `<script src="https://esm.sh/uno"></script>`
				// with `<link rel="stylesheet" href="/@uno.css">`
				if (strings.HasPrefix(srcAttr, "http://") || strings.HasPrefix(srcAttr, "https://")) && strings.HasSuffix(srcAttr, "/uno") {
					unocss = "/@uno.css?ctx=" + btoaUrl(pathname)
					w.Write([]byte(fmt.Sprintf(`<link id="@unocss" rel="stylesheet" href="%s">`, unocss)))
					tok := tokenizer.Next()
					if tok == html.TextToken {
						tokenizer.Next()
					}
					if tok == html.ErrorToken {
						break
					}
					continue
				}
			case "style":
				// strip `<style type="uno/css">...</style>`
				if attrs["type"] == "uno/css" {
					hasInlineUnoConfigCSS = true
					tok := tokenizer.Next()
					if tok == html.TextToken {
						tokenizer.Next()
					}
					if tok == html.ErrorToken {
						break
					}
					continue
				}
			case "link":
				if attrs["rel"] == "stylesheet" {
					if src := attrs["href"]; isAbsPathSpecifier(src) || isRelPathSpecifier(src) {
						cssLinks = append(cssLinks, src)
					}
				}
			}
		}
		w.Write(tokenizer.Raw())
	}
	// reload the page when the html file is modified
	fmt.Fprintf(w, `<script type="module">import createHotContext from"/@hmr";const hot=createHotContext("%s");hot.watch(()=>location.reload());`, pathname)
	if unocss != "" {
		// reload the unocss when the module dependency tree is changed
		fmt.Fprintf(w, `hot.watch("*",(kind,filename)=>{if(/\.(js|mjs|jsx|ts|mts|tsx|vue|svelte)$/i.test(filename)){document.getElementById("@unocss").href="%s&t="+Date.now().toString(36)}});`, unocss)
		// reload the page when the uno.css file is modified
		if !hasInlineUnoConfigCSS {
			u := &url.URL{Path: pathname}
			u = u.ResolveReference(&url.URL{Path: "uno.css"})
			filename := filepath.Join(d.rootDir, u.Path)
			if _, err := os.Stat(filename); err != nil && os.IsNotExist(err) {
				u = &url.URL{Path: "/uno.css"}
				filename = filepath.Join(d.rootDir, u.Path)
			}
			if _, err := os.Stat(filename); err == nil || os.IsExist(err) {
				fmt.Fprintf(w, `hot.watch("%s",()=>location.reload());`, u.Path)
			}
		}
	}
	if len(cssLinks) > 0 {
		// reload the page when the css file is modified
		for _, cssLink := range cssLinks {
			u := &url.URL{Path: pathname}
			u = u.ResolveReference(&url.URL{Path: cssLink})
			fmt.Fprintf(w, `const linkEl=document.querySelector('link[rel="stylesheet"][href="%s"]');hot.watch("%s",(kind,filename)=>{if(kind==="modify")linkEl.href=filename+"?t="+Date.now().toString(36)});`, cssLink, u.Path)
		}
	}
	w.Write([]byte("</script>"))
	fmt.Fprintf(w, `<script>console.log("%%c💚 Built with esm.sh/x, please uncheck \"Disable cache\" in Network tab for better DX!", "color:green")</script>`)
}

func (d *DevServer) ServeModule(w http.ResponseWriter, r *http.Request, pathname string, sourceCode []byte) {
	query := r.URL.Query()
	im, err := atobUrl(query.Get("im"))
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	imHtmlFilename := filepath.Join(d.rootDir, im)
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
					importMap.Src = "file://" + im
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
		fi, err := os.Lstat(filepath.Join(d.rootDir, pathname))
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
	sha := xxhash.New()
	if json.NewEncoder(sha).Encode(importMap) != nil {
		http.Error(w, "Internal Server Error", 500)
		return
	}
	etag := fmt.Sprintf("w/\"%d-%d-%d-%x\"", modTime, size, VERSION, sha.Sum(nil))
	if r.Header.Get("If-None-Match") == etag && !query.Has("t") {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	cacheKey := fmt.Sprintf("module-%s", pathname)
	etagCacheKey := fmt.Sprintf("module-%s.etag", pathname)
	if js, ok := d.loadCache.Load(cacheKey); ok {
		if e, ok := d.loadCache.Load(etagCacheKey); ok {
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
	loader, err := d.getLoader()
	if err != nil {
		fmt.Println(term.Red(err.Error()))
		http.Error(w, "Internal Server Error", 500)
		return
	}
	args := []any{pathname, importMap}
	if sourceCode != nil {
		args = append(args, string(sourceCode))
	}
	_, js, err := loader.Load("module", args)
	if err != nil {
		fmt.Println(term.Red("[loader] " + err.Error()))
		http.Error(w, "Internal Server Error", 500)
		return
	}
	d.loadCache.Store(cacheKey, []byte(js))
	d.loadCache.Store(etagCacheKey, etag)
	header := w.Header()
	header.Set("Content-Type", "application/javascript; charset=utf-8")
	if !query.Has("t") {
		header.Set("Cache-Control", "max-age=0, must-revalidate")
		header.Set("Etag", etag)
	}
	w.Write([]byte(js))
}

func (d *DevServer) ServeCSSModule(w http.ResponseWriter, r *http.Request, pathname string, query url.Values) {
	ret := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:      []string{filepath.Join(d.rootDir, pathname)},
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
	etag := fmt.Sprintf("w/\"%x-%d\"", sha.Sum(nil), VERSION)
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
	w.Write([]byte(`!(new URL(import.meta.url)).searchParams.has("t")&&applyCSS(CSS);`))
	w.Write([]byte(`import createHotContext from"/@hmr";`))
	fmt.Fprintf(w, `createHotContext("%s").accept(m=>applyCSS(m.default));`, pathname)
	w.Write([]byte(`export default CSS;`))
}

func (d *DevServer) ServeUnoCSS(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	ctx, err := atobUrl(query.Get("ctx"))
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	imHtmlFilename := filepath.Join(d.rootDir, ctx)
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
	configFilename := ""
	content := []string{}
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
			case "style":
				for moreAttr {
					var key, val []byte
					key, val, moreAttr = tokenizer.TagAttr()
					if bytes.Equal(key, []byte("type")) && bytes.Equal(val, []byte("uno/css")) {
						tokenizer.Next()
						innerText := bytes.TrimSpace(tokenizer.Text())
						if len(innerText) > 0 {
							configFilename = imHtmlFilename
							configCSS = string(innerText)
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
					content = append(content, string(tokenizer.Text()))
				} else {
					if mainAttr != "" && isHttpSepcifier(srcAttr) {
						if !isHttpSepcifier(mainAttr) && endsWith(mainAttr, supportedModuleExts...) {
							jsEntries[mainAttr] = struct{}{}
						}
					} else if !isHttpSepcifier(srcAttr) && endsWith(srcAttr, supportedModuleExts...) {
						jsEntries[srcAttr] = struct{}{}
					}
				}
			case "link", "meta", "title", "base", "head", "noscript", "slot", "template", "option":
				// ignore
			default:
				content = append(content, string(tokenizer.Raw()))
			}
		}
	}
	if configCSS == "" {
		filename := filepath.Join(filepath.Dir(imHtmlFilename), "uno.css")
		if _, err := os.Stat(filename); err != nil && os.IsNotExist(err) {
			filename = filepath.Join(d.rootDir, "uno.css")
		}
		data, err := os.ReadFile(filename)
		if err != nil && os.IsExist(err) {
			http.Error(w, "Internal Server Error", 500)
			return
		}
		if len(data) > 0 {
			configFilename = filename
			configCSS = string(data)
		}
	}
	contentFiles := map[string]struct{}{}
	for entry := range jsEntries {
		tree, err := d.analyzeDependencyTree(filepath.Join(filepath.Dir(imHtmlFilename), entry), importMap)
		if err == nil {
			for filename, code := range tree {
				if _, ok := contentFiles[filename]; !ok {
					contentFiles[filename] = struct{}{}
					content = append(content, string(code))
				}
			}
		}
	}
	sha := xxhash.New()
	sha.Write([]byte(configCSS))
	for _, s := range content {
		sha.Write([]byte(s))
	}
	etag := fmt.Sprintf("w\"%x\"", sha.Sum(nil))
	if r.Header.Get("If-None-Match") == etag && !query.Has("t") {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	cacheKey := fmt.Sprintf("unocss-%s", ctx)
	etagCacheKey := fmt.Sprintf("unocss-%s.etag", ctx)
	if css, ok := d.loadCache.Load(cacheKey); ok {
		if e, ok := d.loadCache.Load(etagCacheKey); ok {
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
	loader, err := d.getLoader()
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Internal Server Error", 500)
		return
	}
	var config map[string]any = nil
	if configFilename != "" {
		config = map[string]any{
			"filename": configFilename,
			"css":      configCSS,
		}
	}
	_, css, err := loader.Load("unocss", []any{config, strings.Join(content, "\n")})
	if err != nil {
		fmt.Println(err)
		http.Error(w, "Internal Server Error", 500)
		return
	}
	d.loadCache.Store(cacheKey, []byte(css))
	d.loadCache.Store(etagCacheKey, etag)
	header := w.Header()
	header.Set("Content-Type", "text/css; charset=utf-8")
	if !query.Has("t") {
		header.Set("Cache-Control", "max-age=0, must-revalidate")
		header.Set("Etag", etag)
	}
	w.Write([]byte(css))
}

func (d *DevServer) ServeInternalJS(w http.ResponseWriter, r *http.Request, name string) {
	data, err := d.efs.ReadFile("internal/" + name + ".js")
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

func (d *DevServer) ServeHmrWS(w http.ResponseWriter, r *http.Request) {
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
	d.rwlock.Lock()
	d.watchData[conn] = watchList
	d.rwlock.Unlock()
	defer func() {
		d.rwlock.Lock()
		delete(d.watchData, conn)
		d.rwlock.Unlock()
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
					fi, err := os.Lstat(filepath.Join(d.rootDir, filename))
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

func (d *DevServer) analyzeDependencyTree(entry string, importMap common.ImportMap) (tree map[string][]byte, err error) {
	tree = make(map[string][]byte)
	ret := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:      []string{entry},
		Target:           esbuild.ES2022,
		Format:           esbuild.FormatESModule,
		Platform:         esbuild.PlatformBrowser,
		JSX:              esbuild.JSXPreserve,
		Bundle:           true,
		MinifyWhitespace: true,
		Outdir:           "/esbuild",
		Write:            false,
		Plugins: []esbuild.Plugin{
			{
				Name: "loader",
				Setup: func(build esbuild.PluginBuild) {
					build.OnResolve(esbuild.OnResolveOptions{Filter: ".*"}, func(args esbuild.OnResolveArgs) (esbuild.OnResolveResult, error) {
						path, resolved := importMap.Resolve(args.Path)
						if isHttpSepcifier(path) || (!isRelPathSpecifier(path) && !isAbsPathSpecifier(path)) {
							return esbuild.OnResolveResult{Path: path, External: true}, nil
						}
						if resolved {
							if endsWith(path, supportedModuleExts...) {
								return esbuild.OnResolveResult{Path: filepath.Join(d.rootDir, path), Namespace: "module", PluginData: path}, nil
							}
							return esbuild.OnResolveResult{Path: filepath.Join(d.rootDir, path)}, nil
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
							if js, ok := d.loadCache.Load(cacheKey); ok {
								if e, ok := d.loadCache.Load(etagCacheKey); ok {
									if e.(string) == etag {
										contents = string(js.([]byte))
										if lang, ok := d.loadCache.Load(langCacheKey); ok {
											if lang.(string) == "ts" {
												loader = esbuild.LoaderTS
											}
										}
										break
									}
								}
							}
							preloader, err := d.getLoader()
							if err != nil {
								return esbuild.OnLoadResult{}, err
							}
							lang, code, err := preloader.Load(ext[1:], []any{pathname, contents, importMap})
							if err != nil {
								return esbuild.OnLoadResult{}, err
							}
							d.loadCache.Store(cacheKey, []byte(code))
							d.loadCache.Store(etagCacheKey, etag)
							d.loadCache.Store(langCacheKey, lang)
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

func (d *DevServer) watchFS() {
	for {
		time.Sleep(100 * time.Millisecond)
		d.rwlock.RLock()
		for conn, watchList := range d.watchData {
			for pathname, mtime := range watchList {
				filename, _ := utils.SplitByFirstByte(pathname, '?')
				fi, err := os.Lstat(filepath.Join(d.rootDir, filename))
				if err != nil {
					if os.IsNotExist(err) {
						if watchList[pathname] > 0 {
							watchList[pathname] = 0
							d.purgeLoadCache(pathname)
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
		d.rwlock.RUnlock()
	}
}

func (d *DevServer) purgeLoadCache(filename string) {
	d.loadCache.Delete(fmt.Sprintf("module-%s", filename))
	d.loadCache.Delete(fmt.Sprintf("module-%s.etag", filename))
	if strings.HasSuffix(filename, ".vue") || strings.HasSuffix(filename, ".svelte") {
		d.loadCache.Delete(fmt.Sprintf("preload-%s", filename))
		d.loadCache.Delete(fmt.Sprintf("preload-%s.etag", filename))
		d.loadCache.Delete(fmt.Sprintf("preload-%s.lang", filename))
	}
}

func (d *DevServer) getLoader() (loader *LoaderWorker, err error) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if d.loader != nil {
		return d.loader, nil
	}
	loaderJs, err := d.efs.ReadFile("internal/loader.js")
	if err != nil {
		return
	}
	loader = &LoaderWorker{}
	err = loader.Start(d.rootDir, loaderJs)
	if err != nil {
		return nil, err
	}
	d.loader = loader
	return
}
