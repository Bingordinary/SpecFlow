package sharedbinding

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ResolvedRef struct {
	VersionRef       string
	FileRef          string
	Layer            string
	SharedContractID string
	SharedVersion    string
	Content          string
}

func ResolveRef(repoRoot, moduleLayer, ref string) (ResolvedRef, error) {
	versionRef := strings.TrimSpace(ref)
	prefix, expectedVersion, err := splitVersionRef(versionRef)
	if err != nil {
		return ResolvedRef{}, err
	}

	layer, err := layerFromPrefix(prefix)
	if err != nil {
		return ResolvedRef{}, err
	}
	if moduleLayer == "stable" && layer != "stable" {
		return ResolvedRef{}, fmt.Errorf("stable-layer module binding must use an s_ shared ref, got %q", versionRef)
	}

	fileRef := sharedFileRef(prefix, layer)
	contentBytes, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(fileRef)))
	if err != nil {
		return ResolvedRef{}, fmt.Errorf("read shared contract %s: %w", fileRef, err)
	}
	content := string(contentBytes)

	frontmatter, err := parseFrontmatter(content)
	if err != nil {
		return ResolvedRef{}, fmt.Errorf("%s: %w", fileRef, err)
	}

	sharedID := strings.TrimSpace(frontmatter["shared_contract_id"])
	actualLayer := strings.TrimSpace(frontmatter["layer"])
	actualVersion := strings.TrimSpace(frontmatter["shared_version"])
	if sharedID == "" || actualLayer == "" || actualVersion == "" {
		return ResolvedRef{}, fmt.Errorf("%s: missing shared_contract_id/layer/shared_version", fileRef)
	}
	if actualLayer != layer {
		return ResolvedRef{}, fmt.Errorf("%s: frontmatter.layer=%s does not match bound layer %s", fileRef, actualLayer, layer)
	}
	if actualVersion != expectedVersion {
		return ResolvedRef{}, fmt.Errorf("%s: bound version %q does not match frontmatter shared_version %q", fileRef, expectedVersion, actualVersion)
	}

	return ResolvedRef{
		VersionRef:       versionRef,
		FileRef:          fileRef,
		Layer:            actualLayer,
		SharedContractID: sharedID,
		SharedVersion:    actualVersion,
		Content:          content,
	}, nil
}

func splitVersionRef(ref string) (string, string, error) {
	parts := strings.SplitN(ref, "@", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid shared contract ref %q", ref)
	}
	prefix := strings.TrimSpace(parts[0])
	version := strings.TrimSpace(parts[1])
	if prefix == "" || version == "" {
		return "", "", fmt.Errorf("invalid shared contract ref %q", ref)
	}
	return prefix, version, nil
}

func layerFromPrefix(prefix string) (string, error) {
	switch {
	case strings.HasPrefix(prefix, "c_"):
		return "candidate", nil
	case strings.HasPrefix(prefix, "s_"):
		return "stable", nil
	default:
		return "", fmt.Errorf("invalid shared contract ref prefix %q", prefix)
	}
}

func sharedFileRef(prefix, layer string) string {
	return fmt.Sprintf("docs/specs/shared_contracts/%s/%s.md", layer, prefix)
}

func parseFrontmatter(content string) (map[string]string, error) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("missing frontmatter start marker")
	}
	endIdx := -1
	for idx := 1; idx < len(lines); idx++ {
		if strings.TrimSpace(lines[idx]) == "---" {
			endIdx = idx
			break
		}
	}
	if endIdx == -1 {
		return nil, fmt.Errorf("missing frontmatter end marker")
	}

	values := map[string]string{}
	for _, line := range lines[1:endIdx] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	return values, nil
}
