package server

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/esm-dev/esm.sh/server/storage"
	logger "github.com/ije/gox/log"
	"github.com/ije/rex"
)

var (
	log          *logger.Logger
	buildQueue   *BuildQueue
	buildStorage storage.Storage
)

// Serve serves the esm.sh server
func Serve(efs EmbedFS) {
	var (
		cfile string
		debug bool
		err   error
	)

	flag.StringVar(&cfile, "config", "config.json", "the config file path")
	flag.BoolVar(&debug, "debug", false, "run the server in DEUBG mode")
	flag.Parse()

	if !existsFile(cfile) {
		config = *DefaultConfig()
		if cfile != "config.json" {
			fmt.Println("Config file not found, use default config")
		}
	} else {
		c, err := LoadConfig(cfile)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		config = *c
		if debug {
			fmt.Println("Config loaded from", cfile)
		}
	}

	if debug {
		config.LogLevel = "debug"
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		embedFS = &MockEmbedFS{cwd}
	} else {
		os.Setenv("NO_COLOR", "1") // disable log color in production
		embedFS = efs
	}

	log, err = logger.New(fmt.Sprintf("file:%s?buffer=32k&fileDateFormat=20060102", path.Join(config.LogDir, "server.log")))
	if err != nil {
		fmt.Printf("initiate logger: %v\n", err)
		os.Exit(1)
	}
	log.SetLevelByName(config.LogLevel)

	var accessLogger *logger.Logger
	if config.LogDir == "" {
		accessLogger = &logger.Logger{}
	} else {
		accessLogger, err = logger.New(fmt.Sprintf("file:%s?buffer=32k&fileDateFormat=20060102", path.Join(config.LogDir, "access.log")))
		if err != nil {
			log.Fatalf("failed to initialize access logger: %v", err)
		}
	}
	// quite in terminal
	accessLogger.SetQuite(true)

	buildStorage, err = storage.New(&config.Storage)
	if err != nil {
		log.Fatalf("failed to initialize build storage(%s): %v", config.Storage.Type, err)
	}
	log.Debugf("storage initialized, type: %s, endpoint: %s", config.Storage.Type, config.Storage.Endpoint)

	// check nodejs environment
	nodejsInstallDir := os.Getenv("NODE_INSTALL_DIR")
	if nodejsInstallDir == "" {
		nodejsInstallDir = path.Join(config.WorkDir, "nodejs")
	}
	nodeVer, pnpmVer, err := checkNodejs(nodejsInstallDir)
	if err != nil {
		log.Fatalf("nodejs: %v", err)
	}
	log.Debugf("nodejs: v%s, pnpm: %s, registry: %s", nodeVer, pnpmVer, config.NpmRegistry)

	err = buildUnenvNodeRuntime()
	if err != nil {
		log.Fatalf("build unenv node runtime: %v", err)
	}
	log.Debugf("unenv node runtime built with %d dist files", len(unenvNodeRuntimeBulid))

	err = buildNpmReplacements(efs)
	if err != nil {
		log.Fatalf("build npm replacements: %v", err)
	}
	log.Debugf("%d npm repalcements loaded", len(npmReplacements))

	// init cjs lexer
	err = initCJSModuleLexer()
	if err != nil {
		log.Fatalf("failed to initialize cjs_lexer: %v", err)
	}
	log.Debugf("%s initialized", cjsModuleLexerPkg)

	// init build queue
	buildQueue = NewBuildQueue(int(config.BuildConcurrency))

	// set rex middlewares
	rex.Use(
		rex.Logger(log),
		rex.AccessLogger(accessLogger),
		rex.Header("Server", "esm.sh"),
		rex.Optional(rex.Compress(), config.Compress),
		cors(config.CorsAllowOrigins),
		esmRouter(debug),
	)

	// start server
	C := rex.Serve(rex.ServerConfig{
		Port: uint16(config.Port),
		TLS: rex.TLSConfig{
			Port: uint16(config.TlsPort),
			AutoTLS: rex.AutoTLSConfig{
				AcceptTOS: config.TlsPort > 0 && !debug,
				CacheDir:  path.Join(config.WorkDir, "autotls"),
			},
		},
	})
	log.Infof("Server is ready on http://localhost:%d", config.Port)

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP, syscall.SIGABRT)
	select {
	case <-c:
	case err = <-C:
		log.Error(err)
	}

	// release resources
	log.FlushBuffer()
	accessLogger.FlushBuffer()
}

func cors(allowOrigins []string) rex.Handle {
	allowList := NewStringSet(allowOrigins...)
	return func(ctx *rex.Context) any {
		origin := ctx.GetHeader("Origin")
		isOptionsMethod := ctx.R.Method == "OPTIONS"
		h := ctx.W.Header()
		if allowList.Len() > 0 {
			if origin != "" {
				if !allowList.Has(origin) {
					return rex.Status(403, "forbidden")
				}
				setCorsHeaders(h, isOptionsMethod, origin)
			} else if isOptionsMethod {
				// not a preflight request
				return rex.Status(405, "method not allowed")
			}
			appendVaryHeader(h, "Origin")
		} else {
			setCorsHeaders(h, isOptionsMethod, "*")
		}
		if isOptionsMethod {
			return rex.NoContent()
		}
		return nil
	}
}

func setCorsHeaders(h http.Header, isOptionsMethod bool, origin string) {
	h.Set("Access-Control-Allow-Origin", origin)
	if isOptionsMethod {
		h.Set("Access-Control-Allow-Headers", "*")
		h.Set("Access-Control-Max-Age", "86400")
	}
}
