package server

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	logx "github.com/ije/gox/log"
	"github.com/ije/rex"
	"github.com/oschwald/maxminddb-golang"
	"github.com/postui/postdb"
)

var (
	nodeEnv *NodeEnv
	mmdbr   *maxminddb.Reader
	db      *postdb.DB
	log     *logx.Logger
	embedFS *embed.FS
)

// Serve serves esmd server
func Serve(fs *embed.FS) {
	var port int
	var httpsPort int
	var etcDir string
	var domain string
	var cdnDomain string
	var cdnDomainChina string
	var logLevel string
	var isDev bool

	flag.IntVar(&port, "port", 80, "http server port")
	flag.IntVar(&httpsPort, "https-port", 443, "https server port")
	flag.StringVar(&etcDir, "etc-dir", "/usr/local/etc/esmd", "etc dir")
	flag.StringVar(&domain, "domain", "esm.sh", "server domain")
	flag.StringVar(&cdnDomain, "cdn-domain", "", "cdn domain")
	flag.StringVar(&cdnDomainChina, "cdn-domain-china", "", "cdn domain for china")
	flag.StringVar(&logLevel, "log", "info", "log level")
	flag.BoolVar(&isDev, "dev", false, "run server in development mode")
	flag.Parse()

	logDir := "/var/log/esmd"
	if isDev {
		etcDir, _ = filepath.Abs(".dev")
		domain = "localhost"
		cdnDomain = ""
		cdnDomainChina = ""
		logDir = path.Join(etcDir, "log")
		logLevel = "debug"
	}

	var err error
	log, err = logx.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(logDir, "main.log")))
	if err != nil {
		fmt.Printf("initiate logger: %v\n", err)
		os.Exit(1)
	}
	log.SetLevelByName(logLevel)

	accessLogger, err := logx.New(fmt.Sprintf("file:%s?buffer=32k&fileDateFormat=20060102", path.Join(logDir, "access.log")))
	if err != nil {
		log.Fatalf("initiate access logger: %v", err)
	}
	accessLogger.SetQuite(true)

	data, err := ioutil.ReadFile(path.Join(etcDir, "build.ver"))
	if err == nil {
		i, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err == nil && i > 0 {
			buildVersion = i
		}
	}

	storageDir := path.Join(etcDir, "storage")
	ensureDir(path.Join(storageDir, fmt.Sprintf("builds/v%d", buildVersion)))
	ensureDir(path.Join(storageDir, fmt.Sprintf("types/v%d", buildVersion)))
	ensureDir(path.Join(storageDir, "raw"))

	polyfills, err := fs.ReadDir("polyfills")
	if err != nil {
		log.Fatal(err)
	}
	for _, entry := range polyfills {
		filename := path.Join(storageDir, fmt.Sprintf("builds/v%d/_%s", buildVersion, entry.Name()))
		if !fileExists(filename) {
			file, err := fs.Open(fmt.Sprintf("polyfills/%s", entry.Name()))
			if err != nil {
				log.Fatal(err)
			}
			f, err := os.Create(filename)
			if err != nil {
				log.Fatal(err)
			}
			_, err = io.Copy(f, file)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	types, err := fs.ReadDir("types")
	if err != nil {
		log.Fatal(err)
	}
	for _, entry := range types {
		filename := path.Join(storageDir, fmt.Sprintf("types/v%d/_%s", buildVersion, entry.Name()))
		if !fileExists(filename) {
			file, err := fs.Open(fmt.Sprintf("types/%s", entry.Name()))
			if err != nil {
				log.Fatal(err)
			}
			f, err := os.Create(filename)
			if err != nil {
				log.Fatal(err)
			}
			_, err = io.Copy(f, file)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	embedFS = fs

	nodeEnv, err = checkNodeEnv()
	if err != nil {
		log.Fatalf("check nodejs env: %v", err)
	}
	log.Debugf("nodejs v%s installed", nodeEnv.version)

	db, err = postdb.Open(path.Join(etcDir, "esm.db"), 0666)
	if err != nil {
		log.Fatalf("initiate esm.db: %v", err)
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

	registerRoute(storageDir, domain, cdnDomain, cdnDomainChina)

	C := rex.Serve(rex.ServerConfig{
		Port: uint16(port),
		TLS: rex.TLSConfig{
			Port:         uint16(httpsPort),
			AutoRedirect: !isDev,
			AutoTLS: rex.AutoTLSConfig{
				AcceptTOS: !isDev,
				Hosts:     []string{"www." + domain, domain},
				CacheDir:  path.Join(etcDir, "autotls"),
			},
		},
	})

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGKILL, syscall.SIGHUP)

	if isDev {
		log.Debugf("Server ready on http://localhost:%d", port)
	}

	select {
	case <-c:
	case err = <-C:
		log.Error(err)
	}
	log.FlushBuffer()
	accessLogger.FlushBuffer()
	db.Close()
}

func init() {
	log = &logx.Logger{}
	embedFS = &embed.FS{}
}
