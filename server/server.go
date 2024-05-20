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
	buildQueue *BuildQueue
	config     *Config
	cache      storage.Cache
	db         storage.DataBase
	fs         storage.FileSystem
	log        *logger.Logger
)

// Serve serves ESM server
func Serve(efs EmbedFS) {
	var (
		cfile string
		isDev bool
		err   error
	)

	flag.StringVar(&cfile, "config", "config.json", "the config file path")
	flag.BoolVar(&isDev, "dev", false, "to run server in development mode")
	flag.Parse()

	if !existsFile(cfile) {
		config = DefaultConfig()
		fmt.Println("Config file not found, use default config")
	} else {
		config, err = LoadConfig(cfile)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		fmt.Println("Config loaded from", cfile)
	}
	buildQueue = NewBuildQueue(int(config.BuildConcurrency))

	if isDev {
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
		log.Fatalf("init cache(%s): %v", config.Cache, err)
	}

	fs, err = storage.OpenFS(config.Storage)
	if err != nil {
		log.Fatalf("init fs(%s): %v", config.Storage, err)
	}

	db, err = storage.OpenDB(config.Database)
	if err != nil {
		log.Fatalf("init db(%s): %v", config.Database, err)
	}

	err = loadNodeLibs(efs)
	if err != nil {
		log.Fatalf("load node libs: %v", err)
	}
	log.Debugf("%d node libs loaded", len(nodeLibs))

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

	nodejsInstallDir := os.Getenv("NODE_INSTALL_DIR")
	if nodejsInstallDir == "" {
		nodejsInstallDir = path.Join(config.WorkDir, "nodejs")
	}
	nodeVer, pnpmVer, err := checkNodejs(nodejsInstallDir)
	if err != nil {
		log.Fatalf("nodejs: %v", err)
	}
	log.Infof("nodejs: v%s, pnpm: %s, registry: %s", nodeVer, pnpmVer, config.NpmRegistry)

	err = initCJSLexerNodeApp()
	if err != nil {
		log.Fatalf("failed to initialize the cjs_lexer node app: %v", err)
	}
	log.Infof("%s initialized", cjsLexerPkg)

	if !config.DisableCompression {
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
				http.MethodPost,
			},
			ExposedHeaders:   []string{"X-TypeScript-Types"},
			AllowCredentials: false,
		}),
		auth(config.AuthSecret),
		router(),
	)

	C := rex.Serve(rex.ServerConfig{
		Port: uint16(config.Port),
		TLS: rex.TLSConfig{
			Port: uint16(config.TlsPort),
			AutoTLS: rex.AutoTLSConfig{
				AcceptTOS: config.TlsPort > 0 && !isDev,
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

func init() {
	config = &Config{}
	log = &logger.Logger{}
}

func auth(secret string) rex.Handle {
	return func(ctx *rex.Context) interface{} {
		if secret != "" && ctx.R.Header.Get("Authorization") != "Bearer "+secret {
			return rex.Status(401, "Unauthorized")
		}
		return nil
	}
}
