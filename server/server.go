package server

import (
	"embed"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	logx "github.com/ije/gox/log"
	"github.com/ije/rex"
	"github.com/oschwald/maxminddb-golang"
	"github.com/postui/postdb"
)

var (
	config  *Config
	node    *Node
	mmdbr   *maxminddb.Reader
	db      *postdb.DB
	log     *logx.Logger
	embedFS *embed.FS
)

// The config for ESM Server
type Config struct {
	storageDir         string
	yarnCacheDir       string
	domain             string
	cdnDomain          string
	cdnDomainChina     string
	unpkgDomain        string
	cjsLexerServerPort uint16
}

// Serve serves ESM server
func Serve(fs *embed.FS) {
	var port int
	var httpsPort int
	var cjsLexerServerPort int
	var etcDir string
	var yarnCacheDir string
	var domain string
	var cdnDomain string
	var cdnDomainChina string
	var unpkgDomain string
	var logLevel string
	var isDev bool

	flag.IntVar(&port, "port", 80, "http server port")
	flag.IntVar(&httpsPort, "https-port", 443, "https server port")
	flag.IntVar(&cjsLexerServerPort, "cjs-lexer-server-port", 2022, "cjs lexer server port")
	flag.StringVar(&etcDir, "etc-dir", "/usr/local/etc/esmd", "the etc dir to store data")
	flag.StringVar(&yarnCacheDir, "yarn-cache-dir", "", "the cache dir for `yarn add`")
	flag.StringVar(&domain, "domain", "esm.sh", "main domain")
	flag.StringVar(&cdnDomain, "cdn-domain", "", "cdn domain")
	flag.StringVar(&cdnDomainChina, "cdn-domain-china", "", "cdn domain for china")
	flag.StringVar(&unpkgDomain, "unpkg-domain", "", "proxy domain for unpkg.com")
	flag.StringVar(&logLevel, "log", "info", "log level")
	flag.BoolVar(&isDev, "dev", false, "run server in development mode")
	flag.Parse()

	logDir := "/var/log/esmd"
	if isDev {
		etcDir, _ = filepath.Abs(".dev")
		logDir = path.Join(etcDir, "log")
		logLevel = "debug"
		domain = "localhost"
		cdnDomain = ""
		cdnDomainChina = ""
	}

	config = &Config{
		storageDir:         path.Join(etcDir, "storage"),
		yarnCacheDir:       yarnCacheDir,
		domain:             domain,
		cdnDomain:          cdnDomain,
		cdnDomainChina:     cdnDomainChina,
		unpkgDomain:        unpkgDomain,
		cjsLexerServerPort: uint16(cjsLexerServerPort),
	}
	embedFS = fs

	var err error
	log, err = logx.New(fmt.Sprintf("file:%s?buffer=32k", path.Join(logDir, "main.log")))
	if err != nil {
		fmt.Printf("initiate logger: %v\n", err)
		os.Exit(1)
	}
	log.SetLevelByName(logLevel)

	node, err = checkNode()
	if err != nil {
		log.Fatalf("check nodejs env: %v", err)
	}
	log.Debugf("nodejs v%s installed, registry: %s", node.version, node.npmRegistry)

	ensureDir(path.Join(config.storageDir, fmt.Sprintf("builds/v%d", VERSION)))
	ensureDir(path.Join(config.storageDir, fmt.Sprintf("types/v%d", VERSION)))
	ensureDir(path.Join(config.storageDir, "raw"))

	db, err = postdb.Open(path.Join(etcDir, "esm.db"), 0666)
	if err != nil {
		log.Fatalf("initiate esm.db: %v", err)
	}

	polyfills, err := embedFS.ReadDir("embed/polyfills")
	if err != nil {
		log.Fatal(err)
	}
	for _, entry := range polyfills {
		name := entry.Name()
		filename := path.Join(config.storageDir, fmt.Sprintf("builds/v%d/%s", VERSION, name))
		if !fileExists(filename) {
			file, err := embedFS.Open(fmt.Sprintf("embed/polyfills/%s", name))
			if err != nil {
				log.Fatal(err)
			}
			f, err := os.Create(filename)
			if err != nil {
				log.Fatal(err)
			}
			_, err = io.Copy(f, file)
			f.Close()
			if err != nil {
				log.Fatal(err)
			}
			log.Debugf("%s added", name)
		}
	}

	types, err := embedFS.ReadDir("embed/types")
	if err != nil {
		log.Fatal(err)
	}
	for _, entry := range types {
		name := entry.Name()
		filename := path.Join(config.storageDir, fmt.Sprintf("types/v%d/%s", VERSION, name))
		if !fileExists(filename) {
			file, err := embedFS.Open(fmt.Sprintf("embed/types/%s", name))
			if err != nil {
				log.Fatal(err)
			}
			f, err := os.Create(filename)
			if err != nil {
				log.Fatal(err)
			}
			_, err = io.Copy(f, file)
			f.Close()
			if err != nil {
				log.Fatal(err)
			}
			log.Debugf("%s added", name)
		}
	}

	mmdata, err := embedFS.ReadFile("embed/china_ip_list.mmdb")
	if err == nil {
		mmdbr, err = maxminddb.FromBytes(mmdata)
		if err != nil {
			log.Fatal(err)
		}
		log.Debugf("china_ip_list.mmdb applied: %+v", mmdbr.Metadata)
	}

	accessLogger, err := logx.New(fmt.Sprintf("file:%s?buffer=32k&fileDateFormat=20060102", path.Join(logDir, "access.log")))
	if err != nil {
		log.Fatalf("initiate access logger: %v", err)
	}
	accessLogger.SetQuite(true)

	// start cjs lexer server
	go func() {
		for {
			err := startCJSLexerServer(config.cjsLexerServerPort, path.Join(etcDir, "cjxlexer.pid"), isDev)
			if err != nil {
				if err.Error() == "EADDRINUSE" {
					config.cjsLexerServerPort++
				} else {
					log.Errorf("cjs lexer server: %v", err)
				}
			}
		}
	}()

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
		query(),
	)

	C := rex.Serve(rex.ServerConfig{
		Port: uint16(port),
		TLS: rex.TLSConfig{
			Port: uint16(httpsPort),
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
		log.Debugf("Testing page at http://localhost:%d?test", port)
	}

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

func init() {
	config = &Config{
		storageDir:         "/usr/local/etc/esmd/storage",
		yarnCacheDir:       "",
		domain:             "esm.sh",
		cjsLexerServerPort: 2022,
	}
	log = &logx.Logger{}
	embedFS = &embed.FS{}
}
