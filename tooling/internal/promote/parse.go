package promote

import (
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
)

func parseFrontmatter(content string) map[string]string {
	return specpaths.ReadFrontmatterStringMap(content)
}
