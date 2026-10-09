package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
)

// loadInputsManifest reads the --inputs-file manifest: a plain text file with
// one extra read input per line — a file path, a directory, or a logical
// reference. Blank lines are skipped; entries are used verbatim. The manifest
// is scratch input, not evidence: a repository-content manifest could be swept
// into the snapshot, so it is rejected before the file
// is read (see framework/verification_scope.md §Input roles).
func loadInputsManifest(repoRoot, manifestPath string) ([]string, error) {
	if strings.TrimSpace(manifestPath) == "" {
		return nil, errors.New("--inputs-file requires a file path")
	}
	abs, err := filepath.Abs(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("--inputs-file %q: %w", manifestPath, err)
	}
	isContent, err := repofiles.IsRepositoryContent(repoRoot, abs)
	if err != nil {
		return nil, fmt.Errorf("--inputs-file %q: %w", manifestPath, err)
	}
	if isContent {
		return nil, fmt.Errorf("--inputs-file %q is a repository-content file — the input manifest is scratch input, not evidence; keep it under an ignored local-state path such as meta/plan_inputs/ or outside the repository", manifestPath)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("--inputs-file %q: %w", manifestPath, err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var inputs []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		inputs = append(inputs, line)
	}
	return inputs, nil
}
