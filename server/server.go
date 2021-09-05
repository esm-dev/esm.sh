package server

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"

	"esm.sh/server/storage"

	logx "github.com/ije/gox/log"
	"github.com/ije/rex"
	"github.com/oschwald/maxminddb-golang"
)

var (
	config  *Config
	node    *Node
	mmdbr   *maxminddb.Reader
	db      storage.DBConn
	fs      storage.FSConn
	log     *logx.Logger
	embedFS *embed.FS
)

// The config for ESM Server
type Config struct {
	yarnCacheDir       string
	cdnDomain          string
	cdnDomainChina     string
	unpkgDomain        string
	cjsLexerServerPort uint16
}

// Serve serves ESM server
func Serve(efs *embed.FS) {
	embedFS = efs

	var (
		port           int
		httpsPort      int
		dbUrl          string
		fsUrl          string
		cdnDomain      string
		cdnDomainChina string
		unpkgDomain    string
		etcDir         string
		yarnCacheDir   string
		logLevel       string
		isDev          bool
	)
	flag.IntVar(&port, "port", 80, "http server port")
	flag.IntVar(&httpsPort, "https-port", 0, "https(autotls) server port, default is disabled")
	flag.StringVar(&dbUrl, "db", "", "database connection Url")
	flag.StringVar(&fsUrl, "fs", "", "file system connection Url")
	flag.StringVar(&cdnDomain, "cdn-domain", "", "cdn domain")
	flag.StringVar(&cdnDomainChina, "cdn-domain-china", "", "cdn domain for china")
	flag.StringVar(&unpkgDomain, "unpkg-domain", "", "proxy domain for unpkg.com")
	flag.StringVar(&etcDir, "etc-dir", "/usr/local/etc/esmd", "the etc dir to store data")
	flag.StringVar(&yarnCacheDir, "yarn-cache-dir", "", "the cache dir for `yarn add`")
	flag.StringVar(&logLevel, "log", "info", "log level")
	flag.BoolVar(&isDev, "dev", false, "run server in development mode")
	flag.Parse()

	logDir := "/var/log/esmd"
	if isDev {
		etcDir, _ = filepath.Abs(".dev")
		logDir = path.Join(etcDir, "log")
		logLevel = "debug"
		cdnDomain = ""
		cdnDomainChina = ""
	}
	if dbUrl == "" {
		dbUrl = fmt.Sprintf("postdb:%s", path.Join(etcDir, "esm.db"))
	}
	if fsUrl == "" {
		fsUrl = fmt.Sprintf("local:%s", path.Join(etcDir, "storage"))
	}

	config = &Config{
		yarnCacheDir:       yarnCacheDir,
		cdnDomain:          cdnDomain,
		cdnDomainChina:     cdnDomainChina,
		unpkgDomain:        unpkgDomain,
		cjsLexerServerPort: uint16(8088),
	}

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

	fs, err = storage.OpenFS(fsUrl)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	db, err = storage.OpenDB(dbUrl)
	if err != nil {
		log.Fatalf("storage: %v", err)
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
			err := startCJSLexerServer(config.cjsLexerServerPort, path.Join(etcDir, "cjx-lexer.pid"), isDev)
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
		yarnCacheDir:       "",
		cjsLexerServerPort: 8088,
	}
	log = &logx.Logger{}
	embedFS = &embed.FS{}
}
