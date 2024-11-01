package server

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/esm-dev/esm.sh/server/storage"
	logger "github.com/ije/gox/log"
	"github.com/ije/rex"
)

var (
	config     *Config
	log        *logger.Logger
	buildQueue *BuildQueue
	esmStorage storage.Storage
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
		config = DefaultConfig()
		if cfile != "config.json" {
			fmt.Println("Config file not found, use default config")
		}
	} else {
		config, err = LoadConfig(cfile)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
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

	log, err = logger.New(fmt.Sprintf("file:%s?buffer=32k&fileDateFormat=20060102", path.Join(config.LogDir, fmt.Sprintf("main-v%d.log", VERSION))))
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

	esmStorage, err = storage.New(&config.Storage)
	if err != nil {
		log.Fatalf("failed to initialize build storage(%s): %v", config.Storage.Type, err)
	}

	err = loadNodeLibs(efs)
	if err != nil {
		log.Fatalf("load node libs: %v", err)
	}
	log.Debugf("%d node libs loaded", len(nodeLibs))

	err = loadNpmPolyfills(efs)
	if err != nil {
		log.Fatalf("load npm polyfills: %v", err)
	}
	log.Debugf("%d npm polyfills loaded", len(npmPolyfills))

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

	// init cjs lexer
	err = initCJSLexer()
	if err != nil {
		log.Fatalf("failed to initialize cjs_lexer: %v", err)
	}
	log.Debugf("%s initialized", cjsLexerPkg)

	// init build queue
	buildQueue = NewBuildQueue(int(config.BuildConcurrency))

	// set rex middlewares
	rex.Use(
		rex.Logger(log),
		rex.AccessLogger(accessLogger),
		rex.Cors(rex.CorsOptions{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"HEAD", "GET", "POST"},
			ExposedHeaders:   []string{"X-Esm-Path", "X-TypeScript-Types"},
			MaxAge:           86400, // 24 hours
			AllowCredentials: false,
		}),
		rex.Header("Server", "esm.sh"),
		rex.Optional(rex.Compress(), string(config.Compress) != "false"),
		auth(config.AuthSecret),
		routes(debug),
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
