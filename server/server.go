package server

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"esm.sh/server/storage"

	logx "github.com/ije/gox/log"
	"github.com/ije/rex"
)

var (
	cdnDomain  string
	cache      storage.Cache
	db         storage.DB
	fs         storage.FS
	buildQueue storage.Queue
	log        *logx.Logger
	node       *Node
	embedFS    EmbedFS
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
		queueUrl         string
		nodeServices     string
		logLevel         string
		logDir           string
		noCompress       bool
		isDev            bool
	)

	flag.IntVar(&port, "port", 80, "http server port")
	flag.IntVar(&httpsPort, "https-port", 0, "https(autotls) server port, default is disabled")
	flag.StringVar(&cdnDomain, "cdn-domain", "", "cdn domain")
	flag.StringVar(&etcDir, "etc-dir", ".esmd", "etc dir")
	flag.StringVar(&cacheUrl, "cache", "", "cache config, default is 'memory:default'")
	flag.StringVar(&dbUrl, "db", "", "database config, default is 'postdb:[etc-dir]/esm.db'")
	flag.StringVar(&fsUrl, "fs", "", "filesystem config, default is 'local:[etc-dir]/storage'")
	flag.StringVar(&queueUrl, "queue", "", "bulid queue config, default is 'chan:memory'")
	flag.IntVar(&buildConcurrency, "build-concurrency", runtime.NumCPU(), "maximum number of concurrent build task")
	flag.StringVar(&nodeServices, "node-services", "", "node services")
	flag.StringVar(&logDir, "log-dir", "", "log dir")
	flag.StringVar(&logLevel, "log-level", "info", "log level")
	flag.BoolVar(&noCompress, "no-compress", false, "disable compression for text content")
	flag.BoolVar(&isDev, "dev", false, "run server in development mode")
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
	if queueUrl == "" {
		queueUrl = "chan:memory"
	}
	if logDir == "" {
		logDir = path.Join(etcDir, "log")
	}

	if isDev {
		logLevel = "debug"
		cdnDomain = "localhost"
		if port != 80 {
			cdnDomain = fmt.Sprintf("localhost:%d", port)
		}
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		embedFS = &devFS{cwd}
	} else {
		embedFS = efs
	}

	log, err = logx.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(logDir, fmt.Sprintf("main-v%d.log", VERSION))))
	if err != nil {
		fmt.Printf("initiate logger: %v\n", err)
		os.Exit(1)
	}
	log.SetLevelByName(logLevel)
	if !isDev {
		os.Setenv("NO_COLOR", "1") // disable color in production
	}

	nodeInstallDir := os.Getenv("NODE_INSTALL_DIR")
	if nodeInstallDir == "" {
		nodeInstallDir = path.Join(etcDir, "nodejs")
	}
	node, err = checkNode(nodeInstallDir)
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

	buildQueue, err = storage.OpenQueue(queueUrl)
	if err != nil {
		log.Fatalf("init storage(queue,%s): %v", fsUrl, err)
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
	accessLogger.SetQuite(true) // quite in terminal

	// start cjs lexer server
	go func() {
		wd := path.Join(etcDir, "ns")
		if err := ensureDir(wd); err != nil {
			log.Fatal(err)
		}
		services := []string{"esm-node-services"}
		if len(nodeServices) > 0 {
			for _, v := range strings.Split(nodeServices, ",") {
				v = strings.TrimSpace(v)
				if len(v) > 0 {
					services = append(services, v)
				}
			}
		}
		for {
			err := startNodeServices(wd, services)
			if err != nil && err.Error() != "signal: interrupt" {
				log.Warnf("node services exit: %v", err)
			}
		}
	}()

	for i := 0; i < buildConcurrency; i++ {
		go serveBuild()
	}

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
		query(isDev),
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
		log.Debugf("Playground at http://localhost:%d?playground", port)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGHUP)
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

func serveBuild() {
	for {
		if nsReady {
			data, err := buildQueue.Pull()
			if err != nil {
				log.Error("buildQueue.Pull:", err)
				continue
			}
			var task BuildTask
			if json.Unmarshal(data, &task) == nil {
				taskId := task.ID()
				done := make(chan struct{}, 1)
				go func() {
					t := time.Now()
					_, err := task.Build()
					if err != nil {
						db.Put("error-"+taskId, "error", storage.Store{"error": err.Error()})
						log.Errorf("build %s: %v", taskId, err)
					} else {
						db.Delete("error-" + taskId)
						log.Infof("build %s in %v", taskId, time.Since(t))
					}
					done <- struct{}{}
				}()
				select {
				case <-done:
				case <-time.After(10 * time.Minute):
					os.RemoveAll(task.wd) // clean up
					log.Errorf("build %s: timeout", taskId)
				}
				cache.Delete("build-task:" + taskId)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func init() {
	embedFS = &embed.FS{}
	log = &logx.Logger{}
}
