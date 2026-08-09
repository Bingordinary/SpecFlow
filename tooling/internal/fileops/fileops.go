package fileops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func VersionWithBumpPatch(version string) string {
	if version == "" {
		return "0.1.0"
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return version
	}
	major := parts[0]
	minor := parts[1]
	patch := parts[2]

	var patchVal int
	if _, err := fmt.Sscanf(patch, "%d", &patchVal); err != nil {
		return version
	}
	patchVal++
	return fmt.Sprintf("%s.%s.%d", major, minor, patchVal)
}
