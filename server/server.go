package server

import (
	"embed"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/esm-dev/esm.sh/server/config"
	"github.com/esm-dev/esm.sh/server/storage"

	logx "github.com/ije/gox/log"
	"github.com/ije/rex"
)

var (
	cfg         *config.Config
	cache       storage.Cache
	db          storage.DataBase
	fs          storage.FileSystem
	buildQueue  *BuildQueue
	log         *logx.Logger
	embedFS     EmbedFS
	fetchLock   sync.Map
	installLock sync.Map
	purgeTimers sync.Map
)

type EmbedFS interface {
	ReadFile(name string) ([]byte, error)
}

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

	if !fileExists(cfile) {
		cfg = config.Default()
		fmt.Println("Config file not found, use default config")
	} else {
		cfg, err = config.Load(cfile)
		if err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		fmt.Println("Config loaded from", cfile)
	}

	if isDev {
		cfg.LogLevel = "debug"
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		embedFS = &devFS{cwd}
	} else {
		os.Setenv("NO_COLOR", "1") // disable log color in production
		embedFS = efs
	}

	log, err = logx.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(cfg.LogDir, fmt.Sprintf("main-v%d.log", VERSION))))
	if err != nil {
		fmt.Printf("initiate logger: %v\n", err)
		os.Exit(1)
	}
	log.SetLevelByName(cfg.LogLevel)

	nodeInstallDir := os.Getenv("NODE_INSTALL_DIR")
	if nodeInstallDir == "" {
		nodeInstallDir = path.Join(cfg.WorkDir, "nodejs")
	}
	nodeVer, pnpmVer, err := checkNodejs(nodeInstallDir)
	if err != nil {
		log.Fatalf("check nodejs: %v", err)
	}
	if cfg.NpmRegistry == "" {
		output, err := exec.Command("npm", "config", "get", "registry").CombinedOutput()
		if err == nil {
			cfg.NpmRegistry = strings.TrimRight(strings.TrimSpace(string(output)), "/") + "/"
		}
	}
	log.Infof("nodejs v%s installed, registry: %s, pnpm: %s", nodeVer, cfg.NpmRegistry, pnpmVer)

	cache, err = storage.OpenCache(cfg.Cache)
	if err != nil {
		log.Fatalf("init storage(cache,%s): %v", cfg.Cache, err)
	}

	fs, err = storage.OpenFS(cfg.Storage)
	if err != nil {
		log.Fatalf("init storage(fs,%s): %v", cfg.Storage, err)
	}

	db, err = storage.OpenDB(cfg.Database)
	if err != nil {
		log.Fatalf("init storage(db,%s): %v", cfg.Database, err)
	}

	buildQueue = newBuildQueue(int(cfg.BuildConcurrency))

	var accessLogger *logx.Logger
	if cfg.LogDir == "" {
		accessLogger = &logx.Logger{}
	} else {
		accessLogger, err = logx.New(fmt.Sprintf("file:%s?buffer=32k&fileDateFormat=20060102", path.Join(cfg.LogDir, "access.log")))
		if err != nil {
			log.Fatalf("initiate access logger: %v", err)
		}
	}
	accessLogger.SetQuite(true) // quite in terminal

	// start cjs lexer server
	go func() {
		for {
			err := startNodeServices()
			if err != nil && err.Error() != "signal: interrupt" {
				log.Warnf("node services exit: %v", err)
			}
			time.Sleep(time.Second / 10)
		}
	}()

	if !cfg.NoCompress {
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
		auth(cfg.AuthSecret),
		postHandler(),
		getHandler(),
	)

	C := rex.Serve(rex.ServerConfig{
		Port: uint16(cfg.Port),
		TLS: rex.TLSConfig{
			Port: uint16(cfg.TlsPort),
			AutoTLS: rex.AutoTLSConfig{
				AcceptTOS: cfg.TlsPort > 0 && !isDev,
				CacheDir:  path.Join(cfg.WorkDir, "autotls"),
			},
		},
	})

	if isDev {
		log.Debugf("Server is ready on http://localhost:%d", cfg.Port)
		log.Debugf("Testing page at http://localhost:%d?test", cfg.Port)
	} else {
		log.Info("Server is ready")
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP)
	select {
	case <-c:
	case err = <-C:
		log.Error(err)
	}

	// release resources
	kill(nsPidFile)
	db.Close()
	log.FlushBuffer()
	accessLogger.FlushBuffer()
}

func init() {
	embedFS = &embed.FS{}
	log = &logx.Logger{}
}
