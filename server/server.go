package server

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"esm.sh/server/storage"

	logx "github.com/ije/gox/log"
	"github.com/ije/rex"
)

var (
	cache      storage.Cache
	db         storage.DB
	fs         storage.FS
	buildQueue *BuildQueue
	log        *logx.Logger
	node       *Node
	embedFS    EmbedFS
)

var (
	// base path for requests
	basePath string
	// http redrect for URLs not from basepath
	baseRedirect bool
	// server origin
	origin string
)

type EmbedFS interface {
	ReadFile(name string) ([]byte, error)
}

// Serve serves ESM server
func Serve(efs EmbedFS) {
	var (
		port             int
		httpsPort        int
		buildConcurrency int
		etcDir           string
		cacheUrl         string
		dbUrl            string
		fsUrl            string
		logLevel         string
		logDir           string
		npmRegistry      string
		unpkgOrigin      string
		noCompress       bool
		isDev            bool
	)
	flag.IntVar(&port, "port", 80, "http server port")
	flag.IntVar(&httpsPort, "https-port", 0, "https(autotls) server port, default is disabled")
	flag.StringVar(&basePath, "base-path", "", "base path")
	flag.BoolVar(&baseRedirect, "base-redirect", false, "http redrect for URLs not from basepath")
	flag.StringVar(&etcDir, "etc-dir", ".esmd", "etc dir")
	flag.StringVar(&cacheUrl, "cache", "", "cache config, default is 'memory:default'")
	flag.StringVar(&dbUrl, "db", "", "database config, default is 'postdb:[etc-dir]/esm.db'")
	flag.StringVar(&fsUrl, "fs", "", "filesystem config, default is 'local:[etc-dir]/storage'")
	flag.IntVar(&buildConcurrency, "build-concurrency", runtime.NumCPU(), "maximum number of concurrent build task")
	flag.StringVar(&logDir, "log-dir", "", "log dir")
	flag.StringVar(&logLevel, "log-level", "info", "log level")
	flag.BoolVar(&noCompress, "no-compress", false, "disable compression for text content")
	flag.BoolVar(&isDev, "dev", false, "run server in development mode")
	flag.StringVar(&origin, "origin", "", "the server origin, default is the request host")
	flag.StringVar(&npmRegistry, "npm-registry", "", "npm registry")
	flag.StringVar(&unpkgOrigin, "unpkg-origin", "https://unpkg.com/", "unpkg.com origin")

	flag.Parse()

	var err error
	etcDir, err = filepath.Abs(etcDir)
	if err != nil {
		fmt.Printf("bad etc dir: %v\n", err)
		os.Exit(1)
	}

	if cacheUrl == "" {
		cacheUrl = "memory:default"
	}
	if dbUrl == "" {
		dbUrl = fmt.Sprintf("postdb:%s", path.Join(etcDir, "esm.db"))
	}
	if fsUrl == "" {
		fsUrl = fmt.Sprintf("local:%s", path.Join(etcDir, "storage"))
	}
	if logDir == "" {
		logDir = path.Join(etcDir, "log")
	}

	if isDev {
		logLevel = "debug"
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		embedFS = &devFS{cwd}
	} else {
		embedFS = efs
		os.Setenv("NO_COLOR", "1") // disable log color in production
	}

	log, err = logx.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(logDir, fmt.Sprintf("main-v%d.log", VERSION))))
	if err != nil {
		fmt.Printf("initiate logger: %v\n", err)
		os.Exit(1)
	}
	log.SetLevelByName(logLevel)

	nodeInstallDir := os.Getenv("NODE_INSTALL_DIR")
	if nodeInstallDir == "" {
		nodeInstallDir = path.Join(etcDir, "nodejs")
	}
	node, err = checkNode(nodeInstallDir, npmRegistry)
	if err != nil {
		log.Fatalf("check nodejs env: %v", err)
	}
	log.Debugf("nodejs v%s installed, registry: %s, yarn: %s", node.version, node.npmRegistry, node.yarn)

	storage.SetLogger(log)
	storage.SetIsDev(isDev)

	cache, err = storage.OpenCache(cacheUrl)
	if err != nil {
		log.Fatalf("init storage(cache,%s): %v", cacheUrl, err)
	}

	db, err = storage.OpenDB(dbUrl)
	if err != nil {
		log.Fatalf("init storage(db,%s): %v", dbUrl, err)
	}

	fs, err = storage.OpenFS(fsUrl)
	if err != nil {
		log.Fatalf("init storage(fs,%s): %v", fsUrl, err)
	}

	buildQueue = newBuildQueue(buildConcurrency)

	var accessLogger *logx.Logger
	if logDir == "" {
		accessLogger = &logx.Logger{}
	} else {
		accessLogger, err = logx.New(fmt.Sprintf("file:%s?buffer=32k&fileDateFormat=20060102", path.Join(logDir, "access.log")))
		if err != nil {
			log.Fatalf("initiate access logger: %v", err)
		}
	}
	accessLogger.SetQuite(true) // quite in terminal

	// start cjs lexer server
	go func() {
		wd := path.Join(etcDir, "ns")
		err := clearDir(wd)
		if err != nil {
			log.Fatal(err)
		}
		services := []string{"esm-node-services"}
		for {
			ctx, cancel := context.WithCancel(context.Background())
			stopNodeServices = cancel
			err := startNodeServices(ctx, wd, services)
			if err != nil && err.Error() != "signal: interrupt" {
				log.Warnf("node services exit: %v", err)
			}
			time.Sleep(time.Second / 10)
		}
	}()

	if !noCompress {
		rex.Use(rex.Compression())
	}
	rex.Use(
		rex.ErrorLogger(log),
		rex.AccessLogger(accessLogger),
		rex.Header("Server", "esm.sh"),
		rex.Cors(rex.CORS{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{
				http.MethodGet,
			},
			AllowedHeaders:   []string{"*"},
			ExposedHeaders:   []string{"X-TypeScript-Types"},
			AllowCredentials: false,
		}),
		query(unpkgOrigin, isDev),
	)

	C := rex.Serve(rex.ServerConfig{
		Port: uint16(port),
		TLS: rex.TLSConfig{
			Port: uint16(httpsPort),
			AutoTLS: rex.AutoTLSConfig{
				AcceptTOS: httpsPort > 0 && !isDev,
				CacheDir:  path.Join(etcDir, "autotls"),
			},
		},
	})

	if isDev {
		log.Debugf("Server ready on http://localhost:%d", port)
		log.Debugf("Testing page at http://localhost:%d?test", port)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP)
	select {
	case <-c:
	case err = <-C:
		log.Error(err)
	}

	// release resources
	db.Close()
	log.FlushBuffer()
	accessLogger.FlushBuffer()
}

func init() {
	embedFS = &embed.FS{}
	log = &logx.Logger{}
}
