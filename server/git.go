package server

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ije/gox/utils"
)

const ghInstallTimeout = 30 * time.Second

var errRepoTooLarge = errors.New("repo is too large")

type GitRef struct {
	Ref string
	Sha string
}

// list refs of a github repository using `git ls-remote repo`
func listGhRepoRefs(repo string) (refs []GitRef, err error) {
	return listGhRepoRefsContext(context.Background(), repo)
}

func listGhRepoRefsContext(ctx context.Context, repo string) (refs []GitRef, err error) {
	return withCache("git ls-remote "+repo, time.Duration(config.NpmQueryCacheTTL)*time.Second, func() ([]GitRef, string, error) {
		stdout := &bytes.Buffer{}
		errout := &bytes.Buffer{}
		cancelCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		cmd := exec.CommandContext(cancelCtx, "git", "ls-remote", repo)
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
	return ghInstallContext(context.Background(), wd, name, tag)
}

func ghInstallContext(ctx context.Context, wd, name, tag string) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	tooLargeFile := filepath.Join(config.WorkDir, "gh-too-large", url.PathEscape(strings.ToLower(name)))
	if existsFile(tooLargeFile) {
		return errRepoTooLarge
	}

	installCtx, cancel := context.WithTimeout(ctx, ghInstallTimeout)
	defer cancel()
	defer func() {
		if err == nil {
			err = installCtx.Err()
		}
		if errors.Is(err, errRepoTooLarge) {
			recordErr := ensureDir(filepath.Dir(tooLargeFile))
			if recordErr == nil {
				recordErr = os.WriteFile(tooLargeFile, []byte(name+"\n"), 0644)
			}
			if recordErr != nil {
				err = errors.Join(err, fmt.Errorf("record oversized repo: %w", recordErr))
			}
		} else if ctx.Err() != nil {
			err = ctx.Err()
		} else if errors.Is(installCtx.Err(), context.DeadlineExceeded) {
			err = errors.New("github: install timeout after 30 seconds")
		}
		if err != nil {
			// A partial extraction must not be treated as an installed package.
			os.RemoveAll(wd)
		}
	}()

	u, err := url.Parse(fmt.Sprintf("https://codeload.github.com/%s/tar.gz/%s", name, tag))
	if err != nil {
		return
	}
	client := newFetchClient("esmd/"+VERSION, 0)
	res, err := client.FetchWithContext(installCtx, u, nil)
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

	if res.ContentLength > maxPackageTarballSize {
		return errRepoTooLarge
	}
	download := &io.LimitedReader{R: res.Body, N: maxPackageTarballSize + 1}
	unzip, err := gzip.NewReader(&contextReader{ctx: installCtx, reader: download})
	if err != nil {
		return err
	}
	defer unzip.Close()
	unpacked := &io.LimitedReader{R: unzip, N: maxPackageTarballSize + 1}
	err = extractPackageTarContext(installCtx, wd, name, unpacked)
	if err == nil {
		// Read through the gzip trailer and count any remaining archive data.
		_, err = io.Copy(io.Discard, &contextReader{ctx: installCtx, reader: unpacked})
	}
	if download.N == 0 || unpacked.N == 0 {
		err = errRepoTooLarge
	}
	return
}
