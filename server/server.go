package server

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/esm-dev/esm.sh/internal/fetch"
	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
	"github.com/ije/gox/set"
	"golang.org/x/crypto/acme/autocert"
)

// Start starts the esm.sh server
func Start() {
	var cfile string
	var err error

	flag.StringVar(&cfile, "config", "config.json", "the config file path")
	flag.Parse()

	if existsFile(cfile) {
		config, err = LoadConfig(cfile)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		if DEBUG {
			fmt.Printf("%s [info] Config loaded from %s\n", time.Now().Format("2006-01-02 15:04:05"), cfile)
		}
	}

	if DEBUG {
		config.LogLevel = "debug"
	} else {
		// disable log color in release build
		os.Setenv("NO_COLOR", "1")
	}

	logger, err := log.New(fmt.Sprintf("file:%s?buffer=64k&fileDateFormat=20060102&term", path.Join(config.LogDir, "server.log")))
	if err != nil {
		fmt.Println("failed to initialize logger:", err)
		os.Exit(1)
	}
	if os.Getenv("ESMDIR") != "" {
		logger.Term(false)
	}
	logger.SetLevelByName(config.LogLevel)

	accessLogger, err := log.New(fmt.Sprintf("file:%s?buffer=1m&fileDateFormat=20060102", path.Join(config.LogDir, "access.log")))
	if err != nil {
		logger.Fatalf("failed to initialize access logger: %v", err)
	}

	// initialize storage
	esmStorage, err := storage.New(&config.Storage)
	if err != nil {
		logger.Fatalf("failed to initialize storage(%s): %v", config.Storage.Type, err)
	}
	logger.Debugf("storage initialized, type: %s, endpoint: %s", config.Storage.Type, config.Storage.Endpoint)

	// load node runtime in background
	go getNodeRuntimeJS("fs")

	// build the handler chain, the last added handler is called first
	var handler http.Handler = esmRouter(esmStorage, logger)
	handler = esmLegacyRouter(esmStorage, handler)
	if config.CustomLandingPage.Origin != "" {
		handler = customLandingPage(&config.CustomLandingPage, handler)
	}
	if config.Compress {
		handler = withCompress(handler)
	}
	if config.AccessLog {
		handler = withAccessLog(accessLogger, handler)
	}
	handler = corsMiddleware(config.CorsAllowOrigins, handler)
	handler = pprofRouter(handler)
	handler = withRecovery(logger, handler)

	// start the http server
	errCh := make(chan error, 2)
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: handler,
	}
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()
	logger.Infof("Server is ready on http://localhost:%d", config.Port)

	// start the https server with autocert (Let's Encrypt) if the `tlsPort` is set
	if config.TlsPort > 0 && !DEBUG {
		cdnURL, err := url.Parse(config.CdnOrigin)
		if err != nil || cdnURL.Hostname() == "" {
			logger.Fatal("cdnOrigin is required when tlsPort is enabled")
		}
		certManager := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache(path.Join(config.WorkDir, "autotls")),
			HostPolicy: autocert.HostWhitelist(cdnURL.Hostname()),
		}
		httpsServer := &http.Server{
			Addr:      fmt.Sprintf(":%d", config.TlsPort),
			Handler:   handler,
			TLSConfig: certManager.TLSConfig(),
		}
		go func() {
			errCh <- httpsServer.ListenAndServeTLS("", "")
		}()
		logger.Infof("Server is ready on https://localhost:%d", config.TlsPort)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGABRT)
	select {
	case <-c:
	case err = <-errCh:
		logger.Error(err)
	}

	// release resources
	logger.FlushBuffer()
	accessLogger.FlushBuffer()
}

// corsMiddleware returns a middleware that handles CORS requests.
func corsMiddleware(allowOrigins []string, next http.Handler) http.Handler {
	allowList := set.NewReadOnly(allowOrigins...)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		isOptions := r.Method == http.MethodOptions
		h := w.Header()
		if allowList.Len() > 0 {
			if origin != "" {
				if !allowList.Has(origin) {
					writeStatus(w, http.StatusForbidden, "forbidden")
					return
				}
				h.Set("Access-Control-Allow-Origin", origin)
			} else if isOptions {
				writeStatus(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			appendVaryHeader(h, "Origin")
		} else {
			h.Set("Access-Control-Allow-Origin", "*")
		}
		if isOptions {
			h.Set("Access-Control-Allow-Headers", "*")
			h.Set("Access-Control-Max-Age", "86400")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// customLandingPage returns a middleware that serves the custom landing page
// from the configured origin.
func customLandingPage(options *LandingPageOptions, next http.Handler) http.Handler {
	assets := set.New[string]()
	for _, p := range options.Assets {
		assets.Add("/" + strings.TrimPrefix(p, "/"))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && !assets.Has(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		query := r.URL.RawQuery
		if query != "" {
			query = "?" + query
		}
		url, err := r.URL.Parse(options.Origin + r.URL.Path + query)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Invalid url")
			return
		}
		fetchClient := fetch.NewClient(r.UserAgent(), 15, false)
		res, err := fetchClient.Fetch(url, nil)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "Failed to fetch custom landing page")
			return
		}
		defer res.Body.Close()
		h := w.Header()
		etag := res.Header.Get("Etag")
		if etag != "" {
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			h.Set("Etag", etag)
		} else {
			lastModified := res.Header.Get("Last-Modified")
			if lastModified != "" {
				v := r.Header.Get("If-Modified-Since")
				if v != "" {
					timeIfModifiedSince, e1 := time.Parse(http.TimeFormat, v)
					timeLastModified, e2 := time.Parse(http.TimeFormat, lastModified)
					if e1 == nil && e2 == nil && !timeIfModifiedSince.After(timeLastModified) {
						w.WriteHeader(http.StatusNotModified)
						return
					}
				}
				h.Set("Last-Modified", lastModified)
			}
		}
		cacheControl := res.Header.Get("Cache-Control")
		if cacheControl == "" {
			cacheControl = ccMustRevalidate
		}
		h.Set("Cache-Control", cacheControl)
		h.Set("Content-Type", res.Header.Get("Content-Type"))
		io.Copy(w, res.Body)
	})
}
