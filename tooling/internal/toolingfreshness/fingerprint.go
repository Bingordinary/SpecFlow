package toolingfreshness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const toolingGoModPath = "specflow/tooling/go.mod"

var fingerprintRoots = []string{
	"specflow/tooling/cmd",
	"specflow/tooling/internal",
}

var fingerprintSingleFiles = []string{
	"specflow/tooling/go.mod",
	"specflow/tooling/go.sum",
}

func IsToolingRepo(repoRoot string) bool {
	_, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(toolingGoModPath)))
	return err == nil
}

func SourceInputFiles(repoRoot string) ([]string, error) {
	if !IsToolingRepo(repoRoot) {
		return nil, fmt.Errorf("tooling repo marker missing: %s", toolingGoModPath)
	}

	files := []string{}
	for _, relDir := range fingerprintRoots {
		root := filepath.Join(repoRoot, filepath.FromSlash(relDir))
		if _, err := os.Stat(root); err != nil {
			return nil, fmt.Errorf("required tooling source directory missing: %s", relDir)
		}
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(info.Name(), ".go") {
				return nil
			}
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	for _, relPath := range fingerprintSingleFiles {
		path := filepath.Join(repoRoot, filepath.FromSlash(relPath))
		if _, err := os.Stat(path); err == nil {
			files = append(files, relPath)
		}
	}

	sort.Strings(files)
	return dedupeSorted(files), nil
}

func LiveFingerprint(repoRoot string) (string, []string, error) {
	files, err := SourceInputFiles(repoRoot)
	if err != nil {
		return "", nil, err
	}

	hasher := sha256.New()
	for _, relPath := range files {
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relPath)))
		if err != nil {
			return "", nil, fmt.Errorf("read %s: %w", relPath, err)
		}
		hasher.Write([]byte(relPath))
		hasher.Write([]byte{0})
		hasher.Write(content)
		hasher.Write([]byte{0})
	}

	return hex.EncodeToString(hasher.Sum(nil)), files, nil
}

func dedupeSorted(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	result := []string{items[0]}
	for i := 1; i < len(items); i++ {
		if items[i] == items[i-1] {
			continue
		}
		result = append(result, items[i])
	}
	return result
}
