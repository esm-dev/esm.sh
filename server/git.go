package server

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"

	"github.com/ije/gox/utils"
)

type GitRef struct {
	Ref string
	Sha string
}

// list repo refs using `git ls-remote repo`
func listRepoRefs(repo string) (refs []GitRef, err error) {
	return withCache(fmt.Sprintf("git ls-remote %s", repo), time.Duration(config.NpmQueryCacheTTL)*time.Second, func() ([]GitRef, string, error) {
		cmd := exec.Command("git", "ls-remote", repo)
		stdout := bytes.NewBuffer(nil)
		errout := bytes.NewBuffer(nil)
		cmd.Stdout = stdout
		cmd.Stderr = errout
		err = cmd.Run()
		if err != nil {
			if errout.Len() > 0 {
				return nil, "", errors.New(errout.String())
			}
			return nil, "", err
		}
		refs = make([]GitRef, 0)
		r := bufio.NewReader(stdout)
		for {
			var line []byte
			line, err = r.ReadBytes('\n')
			if err == io.EOF {
				err = nil
				break
			}
			if err != nil {
				return nil, "", err
			}
			sha, ref := utils.SplitByLastByte(string(bytes.TrimSpace(line)), '\t')
			refs = append(refs, GitRef{
				Ref: ref,
				Sha: sha,
			})
		}
		return refs, "", nil
	})
}

func ghInstall(wd, name, tag string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	url := fmt.Sprintf("https://codeload.github.com/%s/tar.gz/%s", name, tag)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer res.Body.Close()

	if res.StatusCode == 404 || res.StatusCode == 401 {
		return fmt.Errorf("github: repo \"%s\" or tag \"%s\" not found", name, tag)
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("fetch %s failed: %s", url, res.Status)
	}

	err = extractPackageTarball(wd, name, io.LimitReader(res.Body, 256*MB))
	return
}
