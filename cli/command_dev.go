package cli

import (
	"embed"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ije/gox/term"
)

// Serve a web app in development mode.
func Dev(efs *embed.FS) {
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

	serv := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: &Server{efs: efs, rootDir: rootDir, dev: true},
	}
	ln, err := net.Listen("tcp", serv.Addr)
	if err != nil {
		os.Stderr.WriteString(term.Red(err.Error()))
		return
	}

	fmt.Printf(term.Green("Server is ready on http://localhost:%d\n"), *port)
	err = serv.Serve(ln)
	if err != nil {
		os.Stderr.WriteString(term.Red(err.Error()))
		return
	}
}
