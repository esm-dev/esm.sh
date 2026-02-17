package cli

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/esm-dev/esm.sh/web"
	"github.com/ije/gox/term"
)

const serveHelpMessage = `Serve a nobuild web app with esm.sh CDN, HMR, transforming TS/Vue/Svelte on the fly.

Usage: esm.sh %s [app-dir] [options]

Arguments:
  app-dir      Directory to serve, default is current directory

Options:
  --port       Port to serve on, default is 3000
  --help, -h   Show help message
`

// Serve a web app in development mode.
func serve(dev bool) {
	port := flag.Int("port", 3000, "port to serve on")
	args, help := parseCommandFlags()

	if help {
		if dev {
			fmt.Printf(serveHelpMessage, "dev")
		} else {
			fmt.Printf(serveHelpMessage, "serve")
		}
		return
	}

	var appDir string
	if len(args) > 0 {
		appDir = args[0]
	}

	var err error
	if appDir == "" {
		appDir, err = os.Getwd()
	} else {
		appDir, err = filepath.Abs(appDir)
		if err == nil {
			var fi os.FileInfo
			fi, err = os.Stat(appDir)
			if err == nil && !fi.IsDir() {
				err = fmt.Errorf("stat %s: not a directory", appDir)
			}
		}
	}
	if err != nil {
		os.Stderr.WriteString(term.Red(err.Error()))
		return
	}

	s := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: web.NewHandler(web.Config{AppDir: appDir, Dev: dev}),
	}
	ln, err := net.Listen("tcp", s.Addr)
	if err != nil {
		os.Stderr.WriteString(term.Red(err.Error()))
		return
	}

	fmt.Printf(term.Green("Server is ready on http://localhost:%d\n"), *port)
	err = s.Serve(ln)
	if err != nil {
		os.Stderr.WriteString(term.Red(err.Error()))
		return
	}
}

// Serve serves a web app in production mode.
func Serve() {
	serve(false)
}
