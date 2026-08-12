package specvalidation

import (
	"strings"
)

// startsNewTopLevelSection reports whether a line begins a new top-level
// section of the spec document: a markdown heading ("#" at column 0) or a
// top-level "key:" line. Lines inside the acceptance item set are indented
// or list items and never match. The acceptance_item_set marker itself is
// handled by the caller before this check, so it never terminates a scan.
func startsNewTopLevelSection(line, trimmed string) bool {
	if trimmed == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return false
	}
	if strings.HasPrefix(trimmed, "-") {
		return false
	}
	if strings.HasPrefix(trimmed, "#") {
		return true
	}
	if strings.HasPrefix(trimmed, "acceptance_item_set") {
		return false
	}
	return strings.Contains(trimmed, ":")
}

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

		if startsNewTopLevelSection(line, trimmed) {
			break
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

// ExtractAcceptanceItemIDs returns the id values of all acceptance items,
// in document order. Empty values are skipped. Scanning stops at the first
// top-level section (markdown heading or top-level "key:" line) after the
// acceptance item set.
func ExtractAcceptanceItemIDs(content string) []string {
	var ids []string
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

		if startsNewTopLevelSection(line, trimmed) {
			break
		}

		if strings.HasPrefix(trimmed, "- id:") {
			val := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "- id:")), `"'`)
			if val != "" {
				ids = append(ids, val)
			}
		}
	}

	return ids
}

// ExtractImplementationSurfaces returns the implementation_surface values of
// all acceptance items, in document order. Empty values are skipped; the
// <pending> placeholder is returned as-is (the caller decides how to treat
// it). Scanning stops at the first top-level section (markdown heading or
// top-level "key:" line) after the acceptance item set.
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

		if startsNewTopLevelSection(line, trimmed) {
			break
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
