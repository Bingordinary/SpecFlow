package toolingfreshness

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
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
	layout, err := specflowlayout.Resolve(repoRoot)
	if errors.Is(err, specflowlayout.ErrNotFound) {
		// A non-specFlow directory is only allowed to pass through when the
		// repo root was not explicitly overridden by a caller argument.
		// An explicitly provided --repo-root that does not resolve to a
		// specFlow layout is rejected so the freshness gate cannot be
		// bypassed by injecting a foreign path into the argument list.
		cwdAbs, absErr := filepath.Abs(cwd)
		if absErr != nil || sameDirectory(repoRoot, cwdAbs) {
			return nil
		}
		return fmt.Errorf("--repo-root %q does not point to a specFlow project layout", repoRoot)
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(BuildFingerprint) == "" {
		return fmt.Errorf(
			"specflow binary missing embedded build fingerprint; run `%s`",
			buildReleaseRecoveryCommand(repoRoot, layout),
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
		"stale specflow binary: built_fingerprint=%s live_fingerprint=%s; run `%s`",
		shortFingerprint(report.EmbeddedFingerprint),
		shortFingerprint(report.LiveFingerprint),
		buildReleaseRecoveryCommand(repoRoot, layout),
	)
}

func buildReleaseRecoveryCommand(repoRoot string, layout specflowlayout.Layout) string {
	return fmt.Sprintf(
		"cd %q && go run ./cmd/specflowctl build-release --repo-root %q",
		filepath.Join(repoRoot, filepath.FromSlash(layout.ToolingRoot)),
		repoRoot,
	)
}

func ShouldBypass(args []string) bool {
	if len(args) == 0 {
		return true
	}


	switch args[0] {
	case "-h", "--help", "help", "build-release", "tooling-fingerprint", "doctor", "next", "deps", HiddenBuildFingerprintCommand:
		return true
	default:
		return false
	}
}

// sameDirectory reports whether two paths refer to the same directory.
// Symlinks are resolved through os.Stat/os.SameFile, which works across
// platforms; a path that cannot be stat'd is compared by absolute path.
func sameDirectory(a, b string) bool {
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return os.SameFile(sa, sb)
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
