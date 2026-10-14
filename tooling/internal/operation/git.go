package operation

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
)

// git runs one read-only git command in dir and returns stdout.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// requireWorkTreeTop verifies that repoRoot is the git worktree top level, so
// the repository-relative paths reported by git match the operation state's
// path space.
func requireWorkTreeTop(repoRoot string) error {
	return repofiles.RequireWorkTreeTop(repoRoot)
}

// resolveCommit resolves a git ref to the full commit SHA.
func resolveCommit(repoRoot, ref string) (string, error) {
	out, err := git(repoRoot, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve baseline %q: %w", ref, err)
	}
	sha := strings.TrimSpace(out)
	if sha == "" {
		return "", fmt.Errorf("resolve baseline %q: empty commit sha", ref)
	}
	return sha, nil
}
