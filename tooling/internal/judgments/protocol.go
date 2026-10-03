package judgments

import (
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specflowlayout"
	"os"
	"path/filepath"
)

// Protocol fingerprints the actual instructions deployed in this project.
func Protocol(root string) string {
	layout, err := specflowlayout.Resolve(root)
	if err != nil {
		return Digest([]byte(ProtocolVersion))
	}
	content := []byte(ProtocolVersion)
	for _, name := range []string{"unit_verify_checklist.md", "verification_scope.md", "severity_policy.md", "validation_cache.md", "shared_judgments.md"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(layout.FrameworkRoot), name))
		content = append(content, []byte("\n"+name+"\n")...)
		if err != nil {
			content = append(content, []byte("missing")...)
		} else {
			content = append(content, data...)
		}
	}
	return Digest(content)
}
