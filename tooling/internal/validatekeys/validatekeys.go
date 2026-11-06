// Package validatekeys derives the per-object judgment catalog for unit
// validate. It reads the spec's existing structured fields (acceptance item
// promise/source/affects, promise source/depends_on, section headings) and
// produces stable judgment keys with explicit read sets. It does not judge
// anything and does not write state.
package validatekeys

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
)

// Family classifies a judgment key by the object it judges.
type Family string

const (
	FamilyGraph    Family = "graph"
	FamilyItem     Family = "item"
	FamilyPromise  Family = "promise"
	FamilySection  Family = "section"
	FamilyExternal Family = "external"
	FamilyGlobal   Family = "global"
)

// Key is one independently reusable judgment. Reads is the full set of spec
// or external references the judgment depends on; a snapshot change outside
// this set cannot invalidate the verdict.
type Key struct {
	ID     string   `json:"id"`
	Family Family   `json:"family"`
	Check  string   `json:"check"`
	Reads  []string `json:"reads"`
}

// Section is one heading region of the spec main file.
type Section struct {
	Heading string `json:"heading"`
	Level   int    `json:"level"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Direct  bool   `json:"direct"`
	Parent  string `json:"parent,omitempty"`
}

// Item is one acceptance item's parsed declaration subset.
type Item struct {
	ID                  string   `json:"id"`
	Promise             string   `json:"promise,omitempty"`
	Sources             []string `json:"sources,omitempty"`
	Files               []string `json:"files,omitempty"`
	Rules               []string `json:"rules,omitempty"`
	VerificationSurface string   `json:"verification_surface,omitempty"`
	Start               int      `json:"start"`
	End                 int      `json:"end"`
}

// Promise is one promise entry's parsed declaration subset.
type Promise struct {
	ID        string   `json:"id"`
	Sources   []string `json:"sources,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`
	Start     int      `json:"start"`
	End       int      `json:"end"`
}

// Graph is the parsed dependency graph of one unit spec.
type Graph struct {
	Unit           string          `json:"unit"`
	Sections       []Section       `json:"sections"`
	Items          []Item          `json:"items"`
	Promises       []Promise       `json:"promises"`
	Rules          []string        `json:"rules,omitempty"`
	NonBehavior    map[string]bool `json:"non_behavior,omitempty"`
	UnresolvedRefs []string        `json:"unresolved_refs,omitempty"`
}

// Catalog is the complete judgment key set for a full run.
type Catalog struct {
	Unit     string `json:"unit"`
	Items    int    `json:"items"`
	Promises int    `json:"promises"`
	Sections int    `json:"sections"`
	Keys     []Key  `json:"keys"`
}

// Parse reads a unit spec and returns its dependency graph. It fails closed
// on duplicate item/promise ids, duplicate headings, and unresolved source
// anchors: a key catalog built over an ambiguous graph cannot be trusted.
func Parse(unit, content string) (*Graph, error) {
	g := &Graph{
		Unit:        unit,
		Sections:    parseSections(content),
		NonBehavior: parseNonBehaviorBlocks(content),
	}
	g.Rules = parseFrontmatterList(content, "rule_refs")

	seenHeadings := map[string]bool{}
	for _, section := range g.Sections {
		if seenHeadings[section.Heading] {
			return nil, fmt.Errorf("duplicate heading %q — a section key cannot be located unambiguously", section.Heading)
		}
		seenHeadings[section.Heading] = true
	}

	for _, region := range contenthash.PromiseRegions(content) {
		p := Promise{ID: region.ID, Start: region.Start, End: region.End}
		p.Sources = fieldList(region.Text, "source")
		p.DependsOn = fieldList(region.Text, "depends_on")
		g.Promises = append(g.Promises, p)
	}
	if err := uniqueIDs("promise", promiseIDs(g.Promises)); err != nil {
		return nil, err
	}

	for _, region := range contenthash.AcceptanceItemRegions(content) {
		it := Item{ID: region.ID, Start: region.Start, End: region.End}
		it.Promise = fieldScalar(region.Text, "promise")
		it.Sources = fieldList(region.Text, "source")
		it.VerificationSurface = fieldScalar(region.Text, "verification_surface")
		files, rules, err := affectsFields(region.Text)
		if err != nil {
			return nil, err
		}
		it.Files, it.Rules = files, rules
		g.Items = append(g.Items, it)
	}
	if err := uniqueIDs("acceptance item", itemIDs(g.Items)); err != nil {
		return nil, err
	}

	for _, source := range allSources(g) {
		if !seenHeadings[source] {
			g.UnresolvedRefs = appendUnique(g.UnresolvedRefs, source)
		}
	}
	for heading := range g.NonBehavior {
		if !seenHeadings[heading] {
			g.UnresolvedRefs = appendUnique(g.UnresolvedRefs, heading)
		}
	}
	sort.Strings(g.UnresolvedRefs)
	return g, nil
}

// Catalog derives the spec-derived portion of the full-run key set from the
// parsed graph. The key ids and read refs are deterministic for the same
// graph. The run completes the catalog with one object per derived rule
// reference (gaterun.appendDerivedRuleObjects): a stable global rule applies
// by default and never appears in the spec's declared fields.
func parseSections(content string) []Section {
	lines := strings.Split(content, "\n")
	var sections []Section
	fence := fenceState{}
	for i, line := range lines {
		if fence.active {
			fence.advance(line)
			continue
		}
		if level, heading, ok := headingLine(line); ok {
			sections = append(sections, Section{Heading: heading, Level: level, Start: i + 1, End: len(lines)})
		}
		fence.advance(line)
	}
	for i := range sections {
		for j := i + 1; j < len(sections); j++ {
			if sections[j].Level <= sections[i].Level {
				sections[i].End = sections[j].Start - 1
				break
			}
		}
		for j := i - 1; j >= 0; j-- {
			if sections[j].Level < sections[i].Level {
				sections[i].Parent = sections[j].Heading
				break
			}
		}
	}
	markDirect(lines, sections)
	return sections
}

// markDirect records whether a heading block carries content of its own, as
// opposed to being a container whose body is only deeper headings. The rule
// that every narrative block must be sourced applies only to direct-content
// blocks: a container adds no behavior of its own.
func markDirect(lines []string, sections []Section) {
	for i := range sections {
		s := &sections[i]
		var descendants [][2]int
		for j := i + 1; j < len(sections); j++ {
			c := sections[j]
			if c.Start > s.End {
				break
			}
			if c.Level > s.Level {
				descendants = append(descendants, [2]int{c.Start, c.End})
			}
		}
		for ln := s.Start + 1; ln <= s.End; ln++ {
			if strings.TrimSpace(lines[ln-1]) == "" {
				continue
			}
			inDescendant := false
			for _, r := range descendants {
				if ln >= r[0] && ln <= r[1] {
					inDescendant = true
					break
				}
			}
			if !inDescendant {
				s.Direct = true
				break
			}
		}
	}
}

func headingLine(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(line)
	n := 0
	for n < len(trimmed) && trimmed[n] == '#' {
		n++
	}
	if n < 2 || n > 6 || n >= len(trimmed) || (trimmed[n] != ' ' && trimmed[n] != '\t') {
		return 0, "", false
	}
	heading := strings.TrimSpace(trimmed[n:])
	if heading == "" {
		return 0, "", false
	}
	return n, heading, true
}

type fenceState struct {
	active bool
	char   byte
	length int
}

func (f *fenceState) advance(line string) {
	if f.active {
		if isClosingFence(line, f.char, f.length) {
			f.active = false
		}
		return
	}
	if c, n, ok := fenceInfo(line); ok {
		f.active = true
		f.char = c
		f.length = n
	}
}

func fenceInfo(line string) (byte, int, bool) {
	t := strings.TrimSpace(line)
	if t == "" {
		return 0, 0, false
	}
	c := t[0]
	if c != '`' && c != '~' {
		return 0, 0, false
	}
	n := 0
	for n < len(t) && t[n] == c {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	if c == '`' && strings.ContainsRune(t[n:], '`') {
		return 0, 0, false
	}
	return c, n, true
}

func isClosingFence(line string, char byte, length int) bool {
	t := strings.TrimSpace(line)
	n := 0
	for n < len(t) && t[n] == char {
		n++
	}
	if n < length {
		return false
	}
	return strings.TrimSpace(t[n:]) == ""
}

func parseNonBehaviorBlocks(content string) map[string]bool {
	out := map[string]bool{}
	for _, entry := range parseFrontmatterList(content, "non_behavior_blocks") {
		out[entry] = true
	}
	return out
}

func parseFrontmatterList(content, field string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil
	}
	for i := 1; i < end; i++ {
		line := lines[i]
		key, raw, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != field {
			continue
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			var out []string
			base := leadingSpaces(line)
			for j := i + 1; j < end; j++ {
				if strings.TrimSpace(lines[j]) == "" {
					continue
				}
				if leadingSpaces(lines[j]) <= base {
					break
				}
				if entry, ok := strings.CutPrefix(strings.TrimSpace(lines[j]), "- "); ok {
					out = append(out, strings.Trim(strings.TrimSpace(entry), `"'`))
				}
			}
			return out
		}
		if strings.EqualFold(value, "none") {
			return nil
		}
		return parseInlineList(value)
	}
	return nil
}

func fieldList(region, field string) []string {
	for i, line := range strings.Split(region, "\n") {
		key, raw, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != field {
			continue
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			var out []string
			base := leadingSpaces(line)
			for _, next := range strings.Split(region, "\n")[i+1:] {
				if strings.TrimSpace(next) == "" {
					continue
				}
				if leadingSpaces(next) <= base {
					break
				}
				if entry, ok := strings.CutPrefix(strings.TrimSpace(next), "- "); ok {
					out = append(out, strings.Trim(strings.TrimSpace(entry), `"'`))
				}
			}
			return out
		}
		return parseInlineList(value)
	}
	return nil
}

func fieldScalar(region, field string) string {
	for _, line := range strings.Split(region, "\n") {
		key, raw, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != field {
			continue
		}
		return strings.Trim(strings.TrimSpace(raw), `"'`)
	}
	return ""
}

func affectsFields(region string) (files, rules []string, err error) {
	inAffects := false
	base := 0
	list := ""
	for _, line := range strings.Split(region, "\n") {
		trimmed := strings.TrimSpace(line)
		if !inAffects {
			if trimmed == "affects:" {
				inAffects = true
				base = leadingSpaces(line)
			}
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)
		if indent <= base {
			inAffects = false
			continue
		}
		key, raw, ok := strings.Cut(trimmed, ":")
		if ok && strings.TrimSpace(key) == "appendices" {
			return nil, nil, fmt.Errorf("affects.appendices is removed; declare the evidence section with its Keys block instead")
		}
		if ok && raw == "" {
			list = strings.TrimSpace(key)
			continue
		}
		if entry, ok := strings.CutPrefix(trimmed, "- "); ok {
			value := strings.Trim(strings.TrimSpace(entry), `"'`)
			switch list {
			case "files":
				files = append(files, value)
			case "rules":
				rules = append(rules, value)
			}
		}
	}
	return files, rules, nil
}

func parseInlineList(value string) []string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		if value == "" {
			return nil
		}
		return []string{strings.Trim(value, `"'`)}
	}
	inner := strings.TrimSpace(value[1 : len(value)-1])
	if inner == "" {
		return nil
	}
	parts := strings.Split(inner, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func leadingSpaces(line string) int {
	n := 0
	for n < len(line) && (line[n] == ' ' || line[n] == '\t') {
		n++
	}
	return n
}

func uniqueIDs(kind string, ids []string) error {
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			return fmt.Errorf("duplicate %s id %q", kind, id)
		}
		seen[id] = true
	}
	return nil
}

func allSources(g *Graph) []string {
	var out []string
	for _, p := range g.Promises {
		out = append(out, p.Sources...)
	}
	for _, it := range g.Items {
		out = append(out, it.Sources...)
	}
	return out
}

func itemIDs(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.ID)
	}
	return out
}

func promiseIDs(promises []Promise) []string {
	out := make([]string, 0, len(promises))
	for _, p := range promises {
		out = append(out, p.ID)
	}
	return out
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func appendUnique(values []string, value string) []string {
	if !contains(values, value) {
		return append(values, value)
	}
	return values
}

func itemRef(id string) string         { return "item:" + id }
func promiseRef(id string) string      { return "promise:" + id }
func sectionRef(heading string) string { return "section:" + heading }
func fileRef(path string) string       { return "file:" + path }
func ruleRef(id string) string         { return "rule:" + id }
func providerRef(id string) string     { return "provider:" + id }
func unitRef(unit string) string       { return "unit:" + unit }
