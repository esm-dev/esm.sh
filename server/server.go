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
	"go.etcd.io/bbolt"
)

var (
	buildQueue *BuildQueue
	config     *Config
	log        *logger.Logger
	imDB       *bbolt.DB
	cache      storage.Cache
	db         storage.DataBase
	fs         storage.FileSystem
)

// Serve serves the esm.sh server
func Serve(efs EmbedFS) {
	var (
		cfile string
		debug bool
		err   error
	)

	flag.StringVar(&cfile, "config", "config.json", "the config file path")
	flag.BoolVar(&debug, "debug", false, "to run server in DEUBG mode")
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
	buildQueue = NewBuildQueue(int(config.BuildConcurrency))

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

	log, err = logger.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(config.LogDir, fmt.Sprintf("main-v%d.log", VERSION))))
	if err != nil {
		fmt.Printf("initiate logger: %v\n", err)
		os.Exit(1)
	}
	log.SetLevelByName(config.LogLevel)

	cache, err = storage.OpenCache(config.Cache)
	if err != nil {
		log.Fatalf("open cache(%s): %v", config.Cache, err)
	}

	fs, err = storage.OpenFS(config.Storage)
	if err != nil {
		log.Fatalf("open fs(%s): %v", config.Storage, err)
	}

	db, err = storage.OpenDB(config.Database)
	if err != nil {
		log.Fatalf("open db(%s): %v", config.Database, err)
	}

	imDB, err = bbolt.Open(path.Join(config.WorkDir, "im.db"), 0644, nil)
	if err != nil {
		log.Fatalf("open im.db: %v", err)
	}
	err = imDB.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(keyAlias)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(keyImportMaps)
		return err
	})
	if err != nil {
		log.Fatalf("init im.db: %v", err)
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

	var accessLogger *logger.Logger
	if config.LogDir == "" {
		accessLogger = &logger.Logger{}
	} else {
		accessLogger, err = logger.New(fmt.Sprintf("file:%s?buffer=32k&fileDateFormat=20060102", path.Join(config.LogDir, "access.log")))
		if err != nil {
			log.Fatalf("failed to initialize access logger: %v", err)
		}
	}
	accessLogger.SetQuite(true) // quite in terminal

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

	// set rex middlewares
	rex.Use(
		rex.Logger(log),
		rex.AccessLogger(accessLogger),
		rex.Optional(rex.Compress(), !config.DisableCompression),
		rex.Header("Server", "esm.sh"),
		rex.Cors(rex.CorsOptions{
			AllowedOrigins:   []string{"*"},
			AllowedMethods:   []string{"HEAD", "GET", "POST"},
			ExposedHeaders:   []string{"X-Esm-Path", "X-TypeScript-Types"},
			MaxAge:           86400, // 24 hours
			AllowCredentials: false,
		}),
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
	db.Close()
	log.FlushBuffer()
	accessLogger.FlushBuffer()
}
