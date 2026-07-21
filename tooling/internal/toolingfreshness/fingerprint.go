package toolingfreshness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

var fingerprintRoots = []string{
	"cmd",
	"internal",
}

var fingerprintSingleFiles = []string{
	"go.mod",
	"manifest.tsv",
}

var optionalFingerprintSingleFiles = []string{
	"go.sum",
}

type sourceInput struct {
	repositoryRelative string
	toolingRelative    string
}

func SourceInputFiles(repoRoot string) ([]string, error) {
	inputs, err := sourceInputs(repoRoot)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, len(inputs))
	for _, input := range inputs {
		files = append(files, input.repositoryRelative)
	}
	return files, nil
}

func sourceInputs(repoRoot string) ([]sourceInput, error) {
	layout, err := specflowlayout.Resolve(repoRoot)
	if err != nil {
		return nil, err
	}

	inputs := []sourceInput{}
	for _, relDir := range fingerprintRoots {
		inputFiles, err := sourceFilesUnder(repoRoot, layout.ToolingRoot, relDir, ".go")
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, inputFiles...)
	}

	for _, relPath := range fingerprintSingleFiles {
		repoPath := specflowlayout.Relative(layout.ToolingRoot, relPath)
		path := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
		if _, err := os.Stat(path); err == nil {
			inputs = append(inputs, sourceInput{
				repositoryRelative: repoPath,
				toolingRelative:    relPath,
			})
			continue
		} else {
			return nil, fmt.Errorf("required tooling source file missing: %s", repoPath)
		}
	}

	for _, relPath := range optionalFingerprintSingleFiles {
		repoPath := specflowlayout.Relative(layout.ToolingRoot, relPath)
		path := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
		if _, err := os.Stat(path); err == nil {
			inputs = append(inputs, sourceInput{
				repositoryRelative: repoPath,
				toolingRelative:    relPath,
			})
		}
	}

	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i].toolingRelative < inputs[j].toolingRelative
	})
	return dedupeSortedInputs(inputs), nil
}

func sourceFilesUnder(repoRoot, toolingRoot, relDir, suffix string) ([]sourceInput, error) {
	repoDir := specflowlayout.Relative(toolingRoot, relDir)
	root := filepath.Join(repoRoot, filepath.FromSlash(repoDir))
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("required tooling source directory missing: %s", repoDir)
	}

	files := []sourceInput{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if suffix != "" && !strings.HasSuffix(info.Name(), suffix) {
			return nil
		}
		repoRel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		toolingRel, err := filepath.Rel(filepath.Join(repoRoot, filepath.FromSlash(toolingRoot)), path)
		if err != nil {
			return err
		}
		files = append(files, sourceInput{
			repositoryRelative: filepath.ToSlash(repoRel),
			toolingRelative:    filepath.ToSlash(toolingRel),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func LiveFingerprint(repoRoot string) (string, []string, error) {
	inputs, err := sourceInputs(repoRoot)
	if err != nil {
		return "", nil, err
	}

	hasher := sha256.New()
	files := make([]string, 0, len(inputs))
	for _, input := range inputs {
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(input.repositoryRelative)))
		if err != nil {
			return "", nil, fmt.Errorf("read %s: %w", input.repositoryRelative, err)
		}
		content = normalizeFingerprintContent(content)
		hasher.Write([]byte(input.toolingRelative))
		hasher.Write([]byte{0})
		hasher.Write(content)
		hasher.Write([]byte{0})
		files = append(files, input.repositoryRelative)
	}

	return hex.EncodeToString(hasher.Sum(nil)), files, nil
}

func normalizeFingerprintContent(content []byte) []byte {
	return []byte(specpaths.NormalizeText(string(content)))
}

func dedupeSortedInputs(items []sourceInput) []sourceInput {
	if len(items) == 0 {
		return nil
	}
	result := []sourceInput{items[0]}
	for i := 1; i < len(items); i++ {
		if items[i].toolingRelative == items[i-1].toolingRelative {
			continue
		}
		result = append(result, items[i])
	}
	return result
}
