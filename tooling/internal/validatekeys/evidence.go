package validatekeys

import (
	"fmt"
	"sort"
	"strings"
)

// EvidenceSection is one `##` behavior section of the unit's evidence record.
// The Keys block is the section's formal binding: it names exactly one
// acceptance item, and the content below it is observed behavior prose.
type EvidenceSection struct {
	Base    string `json:"base"`
	Heading string `json:"heading"`
	Item    string `json:"item"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
}

// EvidenceSectionRef is the read-ref spelling that carries one evidence-record
// section. One record, one section per item: the item id identifies the
// section.
func EvidenceSectionRef(itemID string) string {
	return "evidence_section:" + itemID
}

// ParseEvidenceRecord parses every `##` section of the evidence record. Every
// section must start with a non-blank `Keys:` block declaring exactly one
// `item:` key that resolves to an acceptance item of the unit spec, and the
// record carries at most one section per item. The record must contain at
// least one section — an emptied record fails here, which is what forces the
// final-round deletion of the file.
func ParseEvidenceRecord(base, content string, graph *Graph) ([]EvidenceSection, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return nil, fmt.Errorf("evidence record reference is empty")
	}
	lines := strings.Split(content, "\n")
	type heading struct {
		text  string
		index int
	}
	var headings []heading
	fence := fenceState{}
	for i, line := range lines {
		if fence.active {
			fence.advance(line)
			continue
		}
		if level, text, ok := headingLine(line); ok && level == 2 {
			headings = append(headings, heading{text: text, index: i})
		}
		fence.advance(line)
	}
	if len(headings) == 0 {
		return nil, fmt.Errorf("evidence record %q has no ## sections — delete the file when no acceptance item is evidence-driven", base)
	}
	seenItems := map[string]bool{}
	sections := make([]EvidenceSection, 0, len(headings))
	for i, h := range headings {
		end := len(lines)
		if i+1 < len(headings) {
			end = headings[i+1].index
		}
		item, err := parseEvidenceKeys(lines, h.index+1, end, graph)
		if err != nil {
			return nil, fmt.Errorf("evidence record %q section %q: %w", base, h.text, err)
		}
		if seenItems[item] {
			return nil, fmt.Errorf("evidence record %q has more than one section for item %q", base, item)
		}
		seenItems[item] = true
		sections = append(sections, EvidenceSection{Base: base, Heading: h.text, Item: item, Start: h.index + 1, End: end})
	}
	sort.Slice(sections, func(i, j int) bool { return sections[i].Start < sections[j].Start })
	return sections, nil
}

// parseEvidenceKeys reads the section's Keys block and returns the single
// acceptance item id it binds. The vocabulary is closed: only `item:<id>`, and
// exactly one key per section. A second key anywhere in the section is
// rejected, including one written at a different indentation than the block.
func parseEvidenceKeys(lines []string, from, to int, graph *Graph) (string, error) {
	if err := rejectExtraItemKeys(lines, from, to); err != nil {
		return "", err
	}
	i := from
	for i < to && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= to || strings.TrimSpace(lines[i]) != "Keys:" {
		return "", fmt.Errorf("the first non-blank content must be a Keys: block")
	}
	i++
	for i < to {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			i++
			continue
		}
		if leadingSpaces(line) == 0 || !strings.HasPrefix(trimmed, "- ") {
			break
		}
		raw := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
		kind, name, ok := strings.Cut(raw, ":")
		if !ok {
			return "", fmt.Errorf("key %q has no type prefix", raw)
		}
		name = strings.TrimSpace(name)
		if kind != "item" {
			return "", fmt.Errorf("key %q has unknown type %q — an evidence-record section declares exactly one item key", raw, kind)
		}
		if name == "" {
			return "", fmt.Errorf("key %q has an empty name", raw)
		}
		if !graph.HasItem(name) {
			return "", fmt.Errorf("item key %q does not resolve to an acceptance item", name)
		}
		return name, nil
	}
	return "", fmt.Errorf("the Keys: block has no keys")
}

// rejectExtraItemKeys counts every `- item:` line in the section outside code
// fences. The Keys block is the first such line; any later one is a second key
// and is rejected even when its indentation would end the block parse. A prose
// bullet that happens to spell `- item:` is indistinguishable from a key under
// the closed vocabulary, so it is rejected rather than silently ignored.
func rejectExtraItemKeys(lines []string, from, to int) error {
	count := 0
	fence := fenceState{}
	for i := from; i < to; i++ {
		line := lines[i]
		if fence.active {
			fence.advance(line)
			continue
		}
		if _, _, ok := fenceInfo(line); ok {
			fence.advance(line)
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- item:") {
			count++
			if count > 1 {
				return fmt.Errorf("an evidence-record section declares exactly one item key; a second item key was found")
			}
		}
	}
	return nil
}

// HasItem reports whether the graph declares the acceptance item.
func (g *Graph) HasItem(id string) bool {
	for _, item := range g.Items {
		if item.ID == id {
			return true
		}
	}
	return false
}
