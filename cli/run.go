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

func Run(efs *embed.FS) (err error) {
	port := flag.Int("port", 3000, "port to serve on")
	args := parseCommandFlag()

	rootDir := ""
	if len(args) > 0 {
		rootDir = args[0]
	}
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
		return err
	}

	server := &http.Server{Addr: fmt.Sprintf(":%d", *port), Handler: &Server{efs: efs, rootDir: rootDir}}
	ln, err := net.Listen("tcp", server.Addr)
	if err != nil {
		os.Stderr.WriteString(term.Red(err.Error()))
		return err
	}
	fmt.Printf(term.Green("Server is ready on http://localhost:%d\n"), *port)
	return server.Serve(ln)
}
