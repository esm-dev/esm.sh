package server

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	"esm.sh/server/storage"

	logx "github.com/ije/gox/log"
	"github.com/ije/rex"
)

var (
	cdnDomain string
	cache     storage.Cache
	db        storage.DB
	fs        storage.FS
	embedFS   *embed.FS
	log       *logx.Logger
	node      *Node
)

// Serve serves ESM server
func Serve(efs *embed.FS) {
	embedFS = efs

	var (
		port       int
		httpsPort  int
		cacheUrl   string
		dbUrl      string
		fsUrl      string
		etcDir     string
		logLevel   string
		logDir     string
		noCompress bool
		isDev      bool
	)

	flag.IntVar(&port, "port", 80, "http server port")
	flag.IntVar(&httpsPort, "https-port", 0, "https(autotls) server port, default is disabled")
	flag.StringVar(&cacheUrl, "cache", "", "cache connection Url")
	flag.StringVar(&dbUrl, "db", "", "database connection Url")
	flag.StringVar(&fsUrl, "fs", "", "file system connection Url")
	flag.StringVar(&cdnDomain, "cdn-domain", "", "cdn domain")
	flag.StringVar(&etcDir, "etc-dir", "/usr/local/etc/esmd", "the etc dir to store data")
	flag.StringVar(&logLevel, "log-level", "info", "log level")
	flag.StringVar(&logDir, "log-dir", "/var/log/esmd", "the log dir to store server logs")
	flag.BoolVar(&noCompress, "no-compress", false, "disable compression for text content")
	flag.BoolVar(&isDev, "dev", false, "run server in development mode")
	flag.Parse()

	if isDev {
		etcDir, _ = filepath.Abs(".dev")
		logDir = path.Join(etcDir, "log")
		logLevel = "debug"
		cdnDomain = "localhost"
		if port != 80 {
			cdnDomain = fmt.Sprintf("localhost:%d", port)
		}
	}
	if cacheUrl == "" {
		cacheUrl = "memory:main"
	}
	if dbUrl == "" {
		dbUrl = fmt.Sprintf("postdb:%s", path.Join(etcDir, "esm.db"))
	}
	if fsUrl == "" {
		fsUrl = fmt.Sprintf("local:%s", path.Join(etcDir, "storage"))
	}

	var err error
	var log *logx.Logger
	if logDir == "" {
		log = &logx.Logger{}
	} else {
		log, err = logx.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(logDir, "main.log")))
		if err != nil {
			fmt.Printf("initiate logger: %v\n", err)
			os.Exit(1)
		}
	}
	log.SetLevelByName(logLevel)

	nodeInstallDir := os.Getenv("NODE_INSTALL_DIR")
	if nodeInstallDir == "" {
		nodeInstallDir = path.Join(etcDir, "nodejs")
	}
	node, err = checkNode(nodeInstallDir)
	if err != nil {
		log.Fatalf("check nodejs env: %v", err)
	}
	log.Debugf("nodejs v%s installed, registry: %s", node.version, node.npmRegistry)

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

	var accessLogger *logx.Logger
	if logDir == "" {
		accessLogger = &logx.Logger{}
	} else {
		accessLogger, err = logx.New(fmt.Sprintf("file:%s?buffer=32k&fileDateFormat=20060102", path.Join(logDir, "access.log")))
		if err != nil {
			log.Fatalf("initiate access logger: %v", err)
		}
	}
	accessLogger.SetQuite(true)

	// start cjs lexer server
	go func() {
		for {
			err := startCJSLexerServer(path.Join(etcDir, "cjx-lexer.pid"), isDev)
			if err != nil {
				if err.Error() == "EADDRINUSE" {
					cjsLexerServerPort++
				} else {
					log.Errorf("cjs lexer server: %v", err)
				}
			}
		}
	}()

	if !noCompress {
		rex.Use(rex.AutoCompress())
	}
	rex.Use(
		rex.ErrorLogger(log),
		rex.AccessLogger(accessLogger),
		rex.Header("Server", "esm.sh"),
		rex.Cors(rex.CORS{
			AllowAllOrigins: true,
			AllowMethods:    []string{"GET"},
			AllowHeaders:    []string{"Origin", "Content-Type", "Content-Length", "Accept-Encoding"},
			MaxAge:          3600,
		}),
		query(),
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

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL, syscall.SIGHUP)

	if isDev {
		log.Debugf("Server ready on http://localhost:%d", port)
		log.Debugf("Testing page at http://localhost:%d?test", port)
	}

	select {
	case <-c:
	case err = <-C:
		log.Error(err)
	}

	// release resource
	db.Close()
	accessLogger.FlushBuffer()
	log.FlushBuffer()
}

func init() {
	log = &logx.Logger{}
	embedFS = &embed.FS{}
}
