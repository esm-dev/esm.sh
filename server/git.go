package server

import (
	"os/exec"
	"strings"

	"github.com/ije/gox/utils"
)

type GitRef struct {
	Ref string
	Sha string
}

// list repo refs using `git ls-remote repo`
func listRepoRefs(repo string) (refs []GitRef, err error) {
	cmd := exec.Command("git", "ls-remote", repo)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	refs = []GitRef{}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		sha, ref := utils.SplitByLastByte(line, '\t')
		refs = append(refs, GitRef{
			Ref: ref,
			Sha: sha,
		})
	}
	return
}
