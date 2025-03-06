package cli

import "embed"

//go:embed demo
var efs embed.FS

func walkEmbedFS(dir string, callback func(filename string) error) error {
	entries, err := efs.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			err = walkEmbedFS(dir+"/"+entry.Name(), callback)
			if err != nil {
				return err
			}
		} else {
			err = callback(dir + "/" + entry.Name())
			if err != nil {
				return err
			}
		}
	}
	return nil
}
