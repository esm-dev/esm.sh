package server

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"time"

	"github.com/esm-dev/esm.sh/internal/fetch"
	"github.com/ije/gox/utils"
)

type GitRef struct {
	Ref string
	Sha string
}

// list refs of a github repository using `git ls-remote repo`
func listGhRepoRefs(repo string) (refs []GitRef, err error) {
	return withCache("git ls-remote "+repo, time.Duration(config.NpmQueryCacheTTL)*time.Second, func() ([]GitRef, string, error) {
		stdout, recycle := newBuffer()
		defer recycle()
		errout, recycle := newBuffer()
		defer recycle()
		cmd := exec.Command("git", "ls-remote", repo)
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
	u, err := url.Parse(fmt.Sprintf("https://codeload.github.com/%s/tar.gz/%s", name, tag))
	if err != nil {
		return
	}
	client, recycle := fetch.NewClient("esmd/"+VERSION, 30, false)
	defer recycle()
	res, err := client.Fetch(u, nil)
	if err != nil {
		return
	}
	defer res.Body.Close()

	if res.StatusCode == 404 || res.StatusCode == 401 {
		return fmt.Errorf("github: repo \"%s\" or tag \"%s\" not found", name, tag)
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("fetch %s failed: %s", u, res.Status)
	}

	err = extractPackageTarball(wd, name, io.LimitReader(res.Body, maxPackageTarballSize))
	return
}
