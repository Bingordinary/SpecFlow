package specvalidation

import (
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
)

type acceptanceItemFields struct {
	id                    string
	fields                map[string]string
	duplicateFields       []string
	implementationSurface string
	affectsFiles          []string
	affectsEvidenceFiles  []string
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
		item := acceptanceItemFields{id: region.ID, fields: map[string]string{"id": region.ID}}
		fieldIndent := leadingSpaces(region.Text) + 2
		inAffects := false
		listField := ""
		blockField := ""
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
			if blockField != "" {
				if indent > fieldIndent {
					if trimmed != "" {
						item.fields[blockField] += trimmed + "\n"
					}
					continue
				}
				if trimmed == "" {
					continue
				}
				blockField = ""
			}
			switch {
			case indent == fieldIndent && trimmed != "":
				inAffects = false
				listField = ""
				key, raw, ok := strings.Cut(trimmed, ":")
				if !ok || strings.HasPrefix(trimmed, "#") {
					continue
				}
				key = strings.TrimSpace(key)
				if _, exists := item.fields[key]; exists {
					item.duplicateFields = append(item.duplicateFields, key)
				}
				value := acceptanceScalar(raw)
				item.fields[key] = value
				if raw = strings.TrimSpace(raw); strings.HasPrefix(raw, "|") || strings.HasPrefix(raw, ">") {
					if isAcceptanceBlockScalar(value) {
						blockField = key
						item.fields[key] = ""
					}
				}
				inAffects = key == "affects" && value == ""
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
		item.implementationSurface = strings.TrimSpace(item.fields["implementation_surface"])
		items = append(items, item)
	}
	return items
}

// acceptanceScalar reads the scalar notation used by acceptance fields,
// preserving quoted '#' characters while excluding an unquoted inline comment.
func acceptanceScalar(raw string) string {
	value := strings.TrimSpace(raw)
	var quote byte
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if quote != 0 {
			if quote == '"' && ch == '\\' {
				i++
			} else if ch == quote {
				if quote == '\'' && i+1 < len(value) && value[i+1] == '\'' {
					i++
				} else {
					quote = 0
				}
			}
		} else if i == 0 && (ch == '\'' || ch == '"') {
			quote = ch
		} else if ch == '#' && (i == 0 || value[i-1] == ' ' || value[i-1] == '\t') {
			value = strings.TrimSpace(value[:i])
			break
		}
	}
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		return value[1 : len(value)-1]
	}
	if value == "~" || strings.EqualFold(value, "null") {
		return ""
	}
	return value
}

func isAcceptanceBlockScalar(value string) bool {
	if value == "" || (value[0] != '|' && value[0] != '>') {
		return false
	}
	for _, ch := range value[1:] {
		if ch != '+' && ch != '-' && (ch < '1' || ch > '9') {
			return false
		}
	}
	return true
}

func appendAffects(item *acceptanceItemFields, key, value string) {
	if value == "" || value == "none" {
		return
	}
	switch key {
	case "files":
		item.affectsFiles = append(item.affectsFiles, value)
	case "evidence_files":
		item.affectsEvidenceFiles = append(item.affectsEvidenceFiles, value)
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

// ExtractAffectsEvidenceFiles returns the file paths declared in the
// affects.evidence_files blocks of structurally located acceptance items, in
// document order. Evidence files are read-only verification evidence: they
// back the item's judgment but are never part of the unit's implementation
// surface or quality coverage keys (framework/spec_writing_guide.md §7).
func ExtractAffectsEvidenceFiles(content string) []string {
	var files []string
	for _, item := range parseAcceptanceItems(content) {
		files = append(files, item.affectsEvidenceFiles...)
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
