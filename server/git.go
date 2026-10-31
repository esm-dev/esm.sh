package server

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ije/gox/utils"
)

const ghInstallTimeout = 30 * time.Second

var (
	errRepoTooLarge = errors.New("repo is too large")
	gitSema         = make(chan struct{}, 16)
)

type GitRef struct {
	Ref string
	Sha string
}

// list refs of a github repository using `git ls-remote repo`
func listGhRepoRefsContext(ctx context.Context, repo string) (refs []GitRef, err error) {
	return withCache("git ls-remote "+repo, time.Duration(config.NpmQueryCacheTTL)*time.Second, func() ([]GitRef, string, error) {
		stdout := &bytes.Buffer{}
		errout := &bytes.Buffer{}
		select {
		case gitSema <- struct{}{}:
			defer func() { <-gitSema }()
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
		cancelCtx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		// [warning] list only HEAD, branches and tags, the pull request refs of a popular repo take megabytes
		cmd := exec.CommandContext(cancelCtx, "git", "ls-remote", repo, "HEAD", "refs/heads/*", "refs/tags/*")
		// git asks for credentials on missing or private repos, never block on a prompt
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
		cmd.Stdout = stdout
		cmd.Stderr = errout
		err = cmd.Run()
		if err != nil {
			if errout.Len() > 0 {
				// trim the trailing newline, the router matches the " not found" suffix
				return nil, "", errors.New(strings.TrimSpace(errout.String()))
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
			// [warning] an annotated tag points to a tag object, the peeled `^{}` line that follows it has the commit
			if tag, ok := strings.CutSuffix(ref, "^{}"); ok {
				if n := len(refs); n > 0 && refs[n-1].Ref == tag {
					refs[n-1].Sha = sha
				}
				continue
			}
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
	tooLargeKey := "gh-too-large:" + strings.ToLower(name) + "@" + tag
	if negativeCache.get(tooLargeKey) != "" {
		return errRepoTooLarge
	}

	installCtx, cancel := context.WithTimeout(ctx, ghInstallTimeout)
	defer cancel()
	defer func() {
		if err == nil {
			err = installCtx.Err()
		}
		if errors.Is(err, errRepoTooLarge) {
			negativeCache.put(tooLargeKey, errRepoTooLarge.Error(), 0)
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
	err = extractPackageTarballContext(installCtx, wd, name, res.Body)
	if errors.Is(err, errPackageTooLarge) {
		err = errRepoTooLarge
	}
	return
}
