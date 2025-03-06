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

// Serve a web app in development mode.
func Dev() {
	port := flag.Int("port", 3000, "port to serve on")
	appDir, _ := parseCommandFlag()

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
		Handler: web.New(web.Config{AppDir: appDir, Dev: true}),
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
