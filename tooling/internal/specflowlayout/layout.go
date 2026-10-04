package specflowlayout

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	SourceRepo       = "source_repo"
	InstalledProject = "installed_project"
)

var ErrNotFound = errors.New("specFlow layout not found")

type Layout struct {
	Kind          string
	ContentRoot   string
	FrameworkRoot string
	TemplateRoot  string
	ToolingRoot   string
}

func Resolve(repoRoot string) (Layout, error) {
	sourcePresent, err := toolingMarkersPresent(repoRoot, "tooling")
	if err != nil {
		return Layout{}, err
	}
	installedPresent, err := toolingMarkersPresent(repoRoot, "specflow/tooling")
	if err != nil {
		return Layout{}, err
	}

	switch {
	case sourcePresent && installedPresent:
		return Layout{}, fmt.Errorf("ambiguous specFlow layout: both source_repo and installed_project tooling markers are present")
	case sourcePresent:
		return Layout{
			Kind:          SourceRepo,
			ContentRoot:   "",
			FrameworkRoot: "framework",
			TemplateRoot:  "templates",
			ToolingRoot:   "tooling",
		}, nil
	case installedPresent:
		return Layout{
			Kind:          InstalledProject,
			ContentRoot:   "specflow",
			FrameworkRoot: "specflow/framework",
			TemplateRoot:  "specflow/templates",
			ToolingRoot:   "specflow/tooling",
		}, nil
	default:
		return Layout{}, fmt.Errorf("%w: expected tooling/go.mod or tooling/manifest.tsv under source_repo or installed_project roots", ErrNotFound)
	}
}

func Relative(root, path string) string {
	if root == "" {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(root), filepath.FromSlash(path)))
}

func toolingMarkersPresent(repoRoot, toolingRoot string) (bool, error) {
	for _, file := range []string{"go.mod", "manifest.tsv"} {
		path := filepath.Join(repoRoot, filepath.FromSlash(Relative(toolingRoot, file)))
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("inspect specFlow tooling marker %q: %w", path, err)
		}
		if !info.IsDir() {
			return true, nil
		}
	}
	return false, nil
}
