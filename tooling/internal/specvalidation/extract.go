package specvalidation

import (
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
)

type acceptanceItemFields struct {
	id                    string
	implementationSurface string
	affectsFiles          []string
	affectsAppendices     []string
	affectsDependencies   []string
	affectsRules          []string
}

// parseAcceptanceItems is the single semantic parser for acceptance-item
// fields. contenthash owns the structural boundary rules (exact marker,
// headings, item ids, and fences); this package only reads fields inside each
// located item. Public extractors therefore cannot disagree about which item
// set they are reading.
//
// Item fields are read relative to the item's own nesting: the region starts
// at the `- id:` line, its fields sit two columns deeper, and the
// affects.files list two columns deeper again. No document-absolute column is
// assumed, so a consistently nested item block reads the same however far the
// list is indented.
func parseAcceptanceItems(content string) []acceptanceItemFields {
	regions := contenthash.AcceptanceItemRegions(content)
	items := make([]acceptanceItemFields, 0, len(regions))
	for _, region := range regions {
		item := acceptanceItemFields{id: region.ID}
		fieldIndent := leadingSpaces(region.Text) + 2
		inAffects := false
		listField := ""
		fence := acceptanceFence{}
		for _, line := range strings.Split(region.Text, "\n") {
			if fence.active {
				fence.advance(line)
				continue
			}
			if fence.advance(line) {
				continue
			}

			trimmed := strings.TrimSpace(line)
			indent := leadingSpaces(line)
			switch {
			case indent == fieldIndent && strings.HasPrefix(trimmed, "implementation_surface:"):
				value := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "implementation_surface:")), `"'`)
				if value != "" {
					item.implementationSurface = value
				}
				inAffects = false
				listField = ""
			case indent == fieldIndent && trimmed == "affects:":
				inAffects = true
				listField = ""
			case indent == fieldIndent && trimmed != "":
				inAffects = false
				listField = ""
			case inAffects && indent == fieldIndent+2 && strings.Contains(trimmed, ":"):
				key, value, _ := strings.Cut(trimmed, ":")
				listField = key
				for _, entry := range parseInlineRefList(strings.TrimSpace(value)) {
					appendAffects(&item, key, entry)
				}
			case inAffects && listField != "" && indent == fieldIndent+4 && strings.HasPrefix(trimmed, "- "):
				if value := strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")); value != "" {
					appendAffects(&item, listField, strings.Trim(value, `"'`))
				}
			}
		}
		items = append(items, item)
	}
	return items
}

func appendAffects(item *acceptanceItemFields, key, value string) {
	if value == "" || value == "none" {
		return
	}
	switch key {
	case "files":
		item.affectsFiles = append(item.affectsFiles, value)
	case "appendices":
		item.affectsAppendices = append(item.affectsAppendices, value)
	case "dependencies":
		item.affectsDependencies = append(item.affectsDependencies, value)
	case "rules":
		item.affectsRules = append(item.affectsRules, value)
	}
}

// ExtractAffectsDependencies returns the formally declared unit dependencies.
func ExtractAffectsDependencies(content string) []string {
	var refs []string
	for _, item := range parseAcceptanceItems(content) {
		refs = append(refs, item.affectsDependencies...)
	}
	return refs
}

// ExtractAffectsRules returns acceptance-item rule references.
func ExtractAffectsRules(content string) []string {
	var refs []string
	for _, item := range parseAcceptanceItems(content) {
		refs = append(refs, item.affectsRules...)
	}
	return refs
}

func leadingSpaces(line string) int {
	count := 0
	for count < len(line) && line[count] == ' ' {
		count++
	}
	return count
}

// acceptanceFence mirrors the CommonMark fence rules used by contenthash.
// It is kept private because it is an implementation detail of field parsing.
type acceptanceFence struct {
	active bool
	char   byte
	length int
}

// advance consumes line and reports whether it is an opening fence. Closing
// fences are consumed while active and always return false.
func (f *acceptanceFence) advance(line string) bool {
	if f.active {
		if acceptanceFenceCloses(line, f.char, f.length) {
			f.active = false
		}
		return false
	}
	char, length, ok := acceptanceFenceInfo(line)
	if !ok {
		return false
	}
	f.active = true
	f.char = char
	f.length = length
	return true
}

func acceptanceFenceInfo(line string) (byte, int, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0, false
	}
	char := trimmed[0]
	length := 0
	for length < len(trimmed) && trimmed[length] == char {
		length++
	}
	if length < 3 || (char == '`' && strings.ContainsRune(trimmed[length:], '`')) {
		return 0, 0, false
	}
	return char, length, true
}

func acceptanceFenceCloses(line string, char byte, length int) bool {
	trimmed := strings.TrimSpace(line)
	count := 0
	for count < len(trimmed) && trimmed[count] == char {
		count++
	}
	return count >= length && strings.TrimSpace(trimmed[count:]) == ""
}

// ExtractAffectsFiles returns the file paths declared in the affects.files
// blocks of structurally located acceptance items, in document order.
func ExtractAffectsFiles(content string) []string {
	var files []string
	for _, item := range parseAcceptanceItems(content) {
		files = append(files, item.affectsFiles...)
	}
	return files
}

// ExtractAcceptanceItemIDs returns the id values of all acceptance items,
// in document order. Empty values are skipped. The scan is the same
// structural one item-region location uses (contenthash.AcceptanceItemIDs):
// only the exact acceptance_item_set marker line outside a code fence starts
// the set, the set ends at the next top-level heading outside a fence, and a
// fenced `- id:` example is content, not an item — session generation and
// cache-declaration location must share one id space.
func ExtractAcceptanceItemIDs(content string) []string {
	items := parseAcceptanceItems(content)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.id)
	}
	return ids
}

// ExtractImplementationSurfaces returns the implementation_surface values of
// all structurally located acceptance items, in document order. Empty values
// are skipped; the <pending> placeholder is returned as-is (the caller
// decides how to treat it).
func ExtractImplementationSurfaces(content string) []string {
	var surfaces []string
	for _, item := range parseAcceptanceItems(content) {
		if item.implementationSurface != "" {
			surfaces = append(surfaces, item.implementationSurface)
		}
	}
	return surfaces
}

// AcceptanceSurfaces preserves the item-to-file relationship for protection planning.
func AcceptanceSurfaces(content string) map[string][]string {
	out := map[string][]string{}
	for _, item := range parseAcceptanceItems(content) {
		files := append([]string(nil), item.affectsFiles...)
		if item.implementationSurface != "" && item.implementationSurface != SurfacePending {
			files = append(files, item.implementationSurface)
		}
		out[item.id] = files
	}
	return out
}
