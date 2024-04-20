package server

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strings"
	"sync"
	"syscall"

	"github.com/esm-dev/esm.sh/server/config"
	"github.com/esm-dev/esm.sh/server/storage"

	logger "github.com/ije/gox/log"
	"github.com/ije/rex"
)

var (
	cfg          *config.Config
	cache        storage.Cache
	db           storage.DataBase
	fs           storage.FileSystem
	nodeLibs     map[string]string
	buildQueue   *BuildQueue
	log          *logger.Logger
	embedFS      EmbedFS
	fetchLocks   sync.Map
	installLocks sync.Map
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

	if !existsFile(cfile) {
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
	buildQueue = newBuildQueue(int(cfg.BuildConcurrency))

	if isDev {
		cfg.LogLevel = "debug"
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		embedFS = &DevFS{cwd}
	} else {
		os.Setenv("NO_COLOR", "1") // disable log color in production
		embedFS = efs
	}

	log, err = logger.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(cfg.LogDir, fmt.Sprintf("main-v%d.log", VERSION))))
	if err != nil {
		fmt.Printf("initiate logger: %v\n", err)
		os.Exit(1)
	}
	log.SetLevelByName(cfg.LogLevel)

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

	data, err := efs.ReadFile("server/embed/nodelibs.tar.gz")
	if err != nil {
		log.Fatal(err)
	}
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		log.Fatal(err)
	}
	tr := tar.NewReader(gr)
	nodeLibs = make(map[string]string)
	for {
		h, err := tr.Next()
		if err != nil {
			break
		}
		if h.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			if err != nil {
				log.Fatal(err)
			}
			nodeLibs[h.Name] = string(data)
		}
	}
	node_async_hooks_js, err := efs.ReadFile("server/embed/polyfills/node_async_hooks.js")
	if err != nil {
		log.Fatal(err)
	}
	nodeLibs["node/async_hooks.js"] = string(node_async_hooks_js)
	log.Debugf("%d node libs loaded", len(nodeLibs))

	var accessLogger *logger.Logger
	if cfg.LogDir == "" {
		accessLogger = &logger.Logger{}
	} else {
		accessLogger, err = logger.New(fmt.Sprintf("file:%s?buffer=32k&fileDateFormat=20060102", path.Join(cfg.LogDir, "access.log")))
		if err != nil {
			log.Fatalf("initiate access logger: %v", err)
		}
	}
	accessLogger.SetQuite(true) // quite in terminal

	nodejsInstallDir := os.Getenv("NODE_INSTALL_DIR")
	if nodejsInstallDir == "" {
		nodejsInstallDir = path.Join(cfg.WorkDir, "nodejs")
	}
	nodeVer, pnpmVer, err := checkNodejs(nodejsInstallDir)
	if err != nil {
		log.Fatalf("check nodejs: %v", err)
	}
	if cfg.NpmRegistry == "" {
		output, err := exec.Command("npm", "config", "get", "registry").CombinedOutput()
		if err == nil {
			cfg.NpmRegistry = strings.TrimRight(strings.TrimSpace(string(output)), "/") + "/"
		}
	}
	log.Infof("nodejs: v%s, pnpm: %s, registry: %s", nodeVer, pnpmVer, cfg.NpmRegistry)

	err = initCJSLexerWorkDirectory()
	if err != nil {
		log.Fatalf("init cjs-lexer: %v", err)
	}

	if !cfg.DisableCompression {
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
		esmHandler(),
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

	log.Infof("Server is ready on http://localhost:%d", cfg.Port)

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
	embedFS = &embed.FS{}
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
