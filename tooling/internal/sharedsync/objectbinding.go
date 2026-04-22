package sharedsync

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/sharedbinding"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/statusfile"
)

type objectBinding struct {
	Status        statusfile.ObjectStatus
	SharedRefs    []string
	BindingIssues []string
}

func loadObjectBindings(repoRoot, objectType string) (map[string]objectBinding, []string, error) {
	statuses, err := statusfile.LoadObjectStatuses(repoRoot)
	if err != nil {
		return nil, nil, err
	}

	bindings := map[string]objectBinding{}
	unresolvedRefs := []string{}
	for _, status := range statuses {
		if status.ObjectType != objectType {
			continue
		}

		refs, err := readObjectSharedRefs(repoRoot, status)
		if err != nil {
			return nil, nil, err
		}

		bindingIssues := []string{}
		for _, ref := range refs {
			if _, err := sharedbinding.ResolveRef(repoRoot, status.ActiveLayer, ref); err != nil {
				bindingIssues = append(bindingIssues, err.Error())
				unresolvedRefs = append(unresolvedRefs, ref)
			}
		}

		bindings[status.Object] = objectBinding{
			Status:        status,
			SharedRefs:    refs,
			BindingIssues: normalizeStrings(bindingIssues),
		}
	}

	return bindings, normalizeStrings(unresolvedRefs), nil
}

func readObjectSharedRefs(repoRoot string, status statusfile.ObjectStatus) ([]string, error) {
	mainSpecRef, err := specpaths.ObjectMainSpecFileRef(status.ObjectType, status.ActiveLayer, status.Object)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(mainSpecRef)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", mainSpecRef, err)
	}
	_, body, err := parseFrontmatter(string(content))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mainSpecRef, err)
	}
	refs, _, err := parseSharedContractRefs(body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", mainSpecRef, err)
	}
	return normalizeStrings(refs), nil
}

func buildScopeObjects(bindings map[string]objectBinding, sharedFilesByRef map[string]sharedFile, scopedRefs, scopedIDs []string) []string {
	if len(scopedRefs) == 0 && len(scopedIDs) == 0 {
		return nil
	}

	scope := map[string]bool{}
	for object, binding := range bindings {
		if len(selectedSharedRefsForObject(binding.SharedRefs, scopedRefs, scopedIDs, sharedFilesByRef)) > 0 {
			scope[object] = true
		}
	}
	return sortedKeys(scope)
}
