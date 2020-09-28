package server

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	logx "github.com/ije/gox/log"
	"github.com/ije/rex"
	"github.com/postui/postdb"
)

var (
	nodeEnv *NodeEnv
	db      *postdb.DB
)

var (
	log = &logx.Logger{}
)

// Serve serves esmd server
func Serve() {
	var port int
	var httpsPort int
	var etcDir string
	var domain string
	var cdnDomain string
	var logLevel string
	var isDev bool

	flag.IntVar(&port, "port", 80, "http server port")
	flag.IntVar(&httpsPort, "https-port", 443, "https server port")
	flag.StringVar(&etcDir, "etc-dir", "/usr/local/etc/esmd", "etc dir")
	flag.StringVar(&domain, "domain", "esm.sh", "main domain")
	flag.StringVar(&cdnDomain, "cdn-domain", "", "cdn domain")
	flag.StringVar(&logLevel, "log", "info", "log level")
	flag.BoolVar(&isDev, "dev", false, "run server in development mode")
	flag.Parse()

	logDir := "/var/log/esmd"
	if isDev {
		etcDir, _ = filepath.Abs(".dev")
		logDir = path.Join(etcDir, "log")
		logLevel = "debug"
		wd, err := os.Getwd()
		if err == nil {
			data, err := ioutil.ReadFile(path.Join(wd, "README.md"))
			if err == nil {
				readmeMD = string(data)
			}
		}
	}

	storageDir := path.Join(etcDir, "storage")
	ensureDir(path.Join(storageDir, "builds"))
	ensureDir(path.Join(storageDir, "types"))
	ensureDir(path.Join(storageDir, "raw"))

	var err error
	log, err = logx.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(logDir, "main.log")))
	if err != nil {
		fmt.Printf("initiate logger: %v", err)
		os.Exit(1)
	}
	log.SetLevelByName(logLevel)

	accessLogger, err := logx.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(logDir, "access.log")))
	if err != nil {
		log.Fatalf("initiate access logger: %v", err)
	}
	accessLogger.SetQuite(true)

	nodeEnv, err = checkNodeEnv()
	if err != nil {
		log.Fatalf("check nodejs env: %v", err)
	}
	log.Debugf("nodejs v%s installed", nodeEnv.version)

	db, err = postdb.Open(path.Join(etcDir, "esmd.db"), 0666)
	if err != nil {
		log.Fatalf("initiate esmd.db: %v", err)
	}

	rex.Use(
		rex.ErrorLogger(log),
		rex.AccessLogger(accessLogger),
		rex.Header("Server", domain),
		rex.Cors(rex.CORS{
			AllowAllOrigins: true,
			AllowMethods:    []string{"GET", "POST"},
			AllowHeaders:    []string{"Origin", "Content-Type", "Content-Length", "Accept-Encoding", "Authorization"},
			MaxAge:          3600,
		}),
	)

	registerAPI(storageDir, domain, cdnDomain)

	C := rex.Serve(rex.ServerConfig{
		Port: uint16(port),
		TLS: rex.TLSConfig{
			Port:         uint16(httpsPort),
			AutoRedirect: !isDev,
			AutoTLS: rex.AutoTLSConfig{
				AcceptTOS: !isDev,
				Hosts:     []string{"www." + domain, domain, cdnDomain},
				CacheDir:  path.Join(etcDir, "/cache/autotls"),
			},
		},
	})

	if isDev {
		log.Debugf("Server ready on http://localhost:%d", port)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL, syscall.SIGHUP)

	select {
	case err = <-C:
		log.Error(err)
	case s := <-c:
		log.Errorf("exit signal: %v", s)
	}
	db.Close()
}
