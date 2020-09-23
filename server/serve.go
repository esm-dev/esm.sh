package server

import (
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"

	logx "github.com/ije/gox/log"
	"github.com/ije/gox/utils"
	"github.com/ije/rex"
	"github.com/postui/postdb"
)

var (
	db        *postdb.DB
	log       *logx.Logger
	nodeEnv   *NodeEnv
	etcDir    string
	cdnDomain string
)

// Serve serves esmd server
func Serve() {
	var (
		port      int
		httpsPort int
		debug     bool
	)
	flag.IntVar(&port, "port", 80, "http server port")
	flag.IntVar(&httpsPort, "https-port", 443, "https server port")
	flag.StringVar(&etcDir, "etc-dir", "/etc/esmd", "etc dir")
	flag.StringVar(&cdnDomain, "cdn-domain", "", "cdn domain")
	flag.BoolVar(&debug, "debug", false, "run server in debug mode")
	flag.Parse()

	logDir := "/var/log/esmd"
	exename := path.Base(os.Args[0])
	isDev := exename == "main" || exename == "main.exe"
	if isDev {
		debug = true
		etcDir, _ = filepath.Abs("./.dev")
		logDir = path.Join(etcDir, "log")
	}

	ensureDir(path.Join(etcDir, "builds"))
	ensureDir(path.Join(etcDir, "types"))

	var err error
	log, err = logx.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(logDir, "main.log")))
	if err != nil {
		log.Fatalf("initiate logger: %v", err)
	}
	if !debug {
		log.SetLevelByName("info")
		log.SetQuite(true)
	}

	accessLogger, err := logx.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(logDir, "access.log")))
	if err != nil {
		log.Fatalf("initiate access logger: %v", err)
	}
	accessLogger.SetQuite(true)

	nodeEnv, err = checkNodeEnv()
	if err != nil {
		log.Fatalf("check nodejs: %v", err)
	}
	log.Debugf("nodejs installed: v%s", nodeEnv.version)

	db, err = postdb.Open(path.Join(etcDir, "esmd.db"), 0666)
	if err != nil {
		log.Fatalf("initiate esmd.db: %v", err)
	}

	rex.Use(
		rex.ErrorLogger(log),
		rex.AccessLogger(accessLogger),
		rex.Header("Server", "esmd"),
		rex.Cors(rex.CORS{
			AllowAllOrigins: true,
			AllowMethods:    []string{"GET", "POST"},
			AllowHeaders:    []string{"Origin", "Content-Type", "Content-Length", "Accept-Encoding", "Authorization"},
			MaxAge:          3600,
		}),
	)

	rex.Serve(rex.ServerConfig{
		Port: uint16(port),
		TLS: rex.TLSConfig{
			Port:         uint16(httpsPort),
			AutoRedirect: !debug,
			AutoTLS: rex.AutoTLSConfig{
				AcceptTOS: !debug,
				CacheDir:  path.Join(etcDir, "/cache/autotls"),
			},
		},
	})

	// wait exit signal
	utils.WaitExitSignal(func(s os.Signal) bool {
		if db != nil {
			db.Close()
		}
		return true
	})
}
