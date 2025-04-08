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

const serveHelpMessage = "\033[30mesm.sh - A nobuild tool for modern web development.\033[0m" + `

Usage: esm.sh serve [app-dir] [options]

Arguments:
  [app-dir]    Directory to serve, default is current directory

Options:
  --port       Port to serve on, default is 3000
  --help       Show help message
`

// Serve a web app in development mode.
func Serve(dev bool) {
	port := flag.Int("port", 3000, "port to serve on")
	help := flag.Bool("help", false, "port to serve on")
	appDir, _ := parseCommandFlag(2)

	if *help {
		fmt.Print(serveHelpMessage)
		return
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
