package specvalidation

import (
	"strings"
)

// ExtractAffectsFiles returns the file paths declared in the affects.files
// blocks of the acceptance item set, in document order. The scanner uses a
// shared indentation convention with checkAnchors: a 6-space "key:" line
// switches the affects sub-block, and only 8-space-indented "- " entries
// under the files sub-block are collected. Any other "- " line (e.g. the
// next acceptance item's "  - id:") ends the block instead of being
// collected. Scanning stops at the first top-level section after the
// acceptance item set.
func ExtractAffectsFiles(content string) []string {
	var files []string
	lines := strings.Split(content, "\n")
	inAcceptanceBlock := false
	inFilesBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "acceptance_item_set:") {
			inAcceptanceBlock = true
			continue
		}

		if !inAcceptanceBlock {
			continue
		}

		// Stop if we hit a new top-level section
		if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "#") {
			if strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
				if trimmed == "acceptance_item_set:" || strings.HasPrefix(trimmed, "acceptance_item_set") {
					continue
				}
				if !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "- ") {
					break
				}
			}
		}

		// A 6-space-indented "key:" line switches the affects sub-block
		// (files/appendices/rules/dependencies). Only the files sub-block
		// collects anchors; any other sub-block ends it.
		if strings.HasPrefix(line, "      ") && strings.HasSuffix(trimmed, ":") {
			inFilesBlock = trimmed == "files:"
			continue
		}

		if inFilesBlock {
			// Only 8-space-indented "- " entries belong to the files block.
			// Any other "- " line (e.g. the next acceptance item's "  - id:")
			// ends the block instead of being collected as an anchor.
			if strings.HasPrefix(line, "        ") && strings.HasPrefix(trimmed, "- ") {
				fpath := trimmed[2:]
				if fpath != "" {
					files = append(files, fpath)
				}
			} else if trimmed != "" && !strings.HasPrefix(line, "        ") {
				inFilesBlock = false
			}
		}
	}

	return files
}

// ExtractImplementationSurfaces returns the implementation_surface values of
// all acceptance items, in document order. Empty values are skipped; the
// <pending> placeholder is returned as-is (the caller decides how to treat
// it). Scanning stops at the first top-level section after the acceptance
// item set.
func ExtractImplementationSurfaces(content string) []string {
	var surfaces []string
	lines := strings.Split(content, "\n")
	inAcceptanceBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.Contains(trimmed, "acceptance_item_set:") {
			inAcceptanceBlock = true
			continue
		}

		if !inAcceptanceBlock {
			continue
		}

		// Stop if we hit a new top-level section
		if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "#") {
			if strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "-") {
				if trimmed == "acceptance_item_set:" || strings.HasPrefix(trimmed, "acceptance_item_set") {
					continue
				}
				if !strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "- ") {
					break
				}
			}
		}

		if strings.HasPrefix(trimmed, "implementation_surface:") {
			val := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "implementation_surface:")), `"'`)
			if val != "" {
				surfaces = append(surfaces, val)
			}
		}
	}

	return surfaces
}
