package toolingfreshness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const HiddenBuildFingerprintCommand = "__print-build-fingerprint"

var BuildFingerprint = ""

type Report struct {
	EmbeddedFingerprint string
	LiveFingerprint     string
	SourceFiles         []string
}

func (r Report) Fresh() bool {
	return r.EmbeddedFingerprint != "" && r.EmbeddedFingerprint == r.LiveFingerprint
}

func CheckProcess(args []string, cwd string) error {
	if len(args) == 0 || ShouldBypass(args) {
		return nil
	}

	repoRoot, err := ResolveRepoRoot(args, cwd)
	if err != nil {
		return err
	}
	if !IsToolingRepo(repoRoot) {
		return nil
	}
	if strings.TrimSpace(BuildFingerprint) == "" {
		return fmt.Errorf(
			"specflow binary missing embedded build fingerprint; run `go run ./specflow/tooling/cmd/specflowctl build-release --repo-root %s`",
			repoRoot,
		)
	}

	report, err := Compare(repoRoot, BuildFingerprint)
	if err != nil {
		return err
	}
	if report.Fresh() {
		return nil
	}

	return fmt.Errorf(
		"stale specflow binary: built_fingerprint=%s live_fingerprint=%s; run `go run ./specflow/tooling/cmd/specflowctl build-release --repo-root %s`",
		shortFingerprint(report.EmbeddedFingerprint),
		shortFingerprint(report.LiveFingerprint),
		repoRoot,
	)
}

func ShouldBypass(args []string) bool {
	if len(args) == 0 {
		return true
	}

	switch args[0] {
	case "-h", "--help", "help", "build-release", "doctor", HiddenBuildFingerprintCommand:
		return true
	default:
		return false
	}
}

func ResolveRepoRoot(args []string, cwd string) (string, error) {
	repoRoot := cwd
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--repo-root":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--repo-root requires a value")
			}
			repoRoot = args[i+1]
			i++
		case strings.HasPrefix(arg, "--repo-root="):
			repoRoot = strings.TrimPrefix(arg, "--repo-root=")
		}
	}
	return filepath.Abs(repoRoot)
}

func Compare(repoRoot, embeddedFingerprint string) (Report, error) {
	liveFingerprint, files, err := LiveFingerprint(repoRoot)
	if err != nil {
		return Report{}, err
	}
	return Report{
		EmbeddedFingerprint: strings.TrimSpace(embeddedFingerprint),
		LiveFingerprint:     liveFingerprint,
		SourceFiles:         files,
	}, nil
}

func PrintBuildFingerprint() string {
	return strings.TrimSpace(BuildFingerprint)
}

func ReadBuildFingerprintFromBinary(binaryPath string) (string, error) {
	cmd := exec.Command(binaryPath, HiddenBuildFingerprintCommand)
	cmd.Env = append(os.Environ(), "GOCACHE=/tmp/go-build-cache")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read build fingerprint from %s: %v: %s", binaryPath, err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func shortFingerprint(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 12 {
		return value
	}
	return value[:12]
}
