package promote

import (
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/fileops"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

func parseFrontmatter(content string) map[string]string {
	return specpaths.ReadFrontmatterStringMap(content)
}

func copyWithLayerTransform(src, dst string) error {
	return fileops.CopyFileTransform(src, dst, "candidate", "stable")
}

func copyFileTransform(src, dst, fromLayer, toLayer string) error {
	return fileops.CopyFileTransform(src, dst, fromLayer, toLayer)
}
