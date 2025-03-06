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

// Serve a web app.
func Serve() {
	port := flag.Int("port", 3000, "port to serve on")
	rootDir, _ := parseCommandFlag()

	var err error
	if rootDir == "" {
		rootDir, err = os.Getwd()
	} else {
		rootDir, err = filepath.Abs(rootDir)
		if err == nil {
			var fi os.FileInfo
			fi, err = os.Stat(rootDir)
			if err == nil && !fi.IsDir() {
				err = fmt.Errorf("stat %s: not a directory", rootDir)
			}
		}
	}
	if err != nil {
		os.Stderr.WriteString(term.Red(err.Error()))
		return
	}

	s := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: web.New(web.Config{RootDir: rootDir}),
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
