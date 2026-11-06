package validatekeys

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Contract is the machine-readable structure of the unit validate gate. The
// judgment standard stays in framework/unit_validate_checklist.md; this type
// only says which checks exist, how they group, and which objects and reads
// each check produces.
type Contract struct {
	Version int     `yaml:"version"`
	Groups  []Group `yaml:"groups"`
	Checks  []Check `yaml:"checks"`
}

// Group is one coverage group and its checks, in execution order.
type Group struct {
	ID     string   `yaml:"id"`
	Checks []string `yaml:"checks"`
}

// Check is one validate check and the object units it judges.
type Check struct {
	ID    string `yaml:"id"`
	Group string `yaml:"group"`
	Units []Unit `yaml:"units"`
}

// Unit is one object-generating unit of a check. Kind selects the object
// family; Suffix names the judgment; Reads is the dependency surface.
type Unit struct {
	Kind   string   `yaml:"kind"`
	Key    string   `yaml:"key,omitempty"`
	Suffix string   `yaml:"suffix,omitempty"`
	Level  int      `yaml:"level,omitempty"`
	When   string   `yaml:"when,omitempty"`
	Reads  []string `yaml:"reads"`
}

// CatalogInput supplies the run-dependent references the spec graph does not
// contain: derived/global rule ids, the unit's evidence-record base name
// (empty when the unit carries no record), and the parsed evidence sections
// with their item bindings.
type CatalogInput struct {
	Unit             string
	Rules            []string
	EvidenceBase     string
	EvidenceSections []EvidenceSection
}

//go:generate cp ../../../framework/unit_validate_checks.yaml contract.yaml

//go:embed contract.yaml
var embeddedContract []byte

var defaultContract struct {
	once sync.Once
	c    *Contract
	err  error
}

// DefaultContract returns the embedded contract. The embedded copy is kept in
// sync with framework/unit_validate_checks.yaml by a test; a missing or
// invalid contract is a build defect, not a project-state condition.
func DefaultContract() (*Contract, error) {
	defaultContract.once.Do(func() {
		defaultContract.c, defaultContract.err = LoadContract(embeddedContract)
	})
	return defaultContract.c, defaultContract.err
}

// LoadContract parses and validates a contract document.
func LoadContract(data []byte) (*Contract, error) {
	var contract Contract
	if err := yaml.Unmarshal(data, &contract); err != nil {
		return nil, fmt.Errorf("parse validate contract: %w", err)
	}
	if contract.Version != 1 {
		return nil, fmt.Errorf("validate contract version %d is unsupported", contract.Version)
	}
	if len(contract.Groups) == 0 || len(contract.Checks) == 0 {
		return nil, fmt.Errorf("validate contract has no groups or checks")
	}
	groups := map[string]bool{}
	checkGroup := map[string]string{}
	for _, group := range contract.Groups {
		if group.ID == "" || groups[group.ID] {
			return nil, fmt.Errorf("validate contract has an empty or duplicate group %q", group.ID)
		}
		groups[group.ID] = true
		for _, check := range group.Checks {
			if checkGroup[check] != "" {
				return nil, fmt.Errorf("check %q is listed in more than one group", check)
			}
			checkGroup[check] = group.ID
		}
	}
	seenCheck := map[string]bool{}
	for _, check := range contract.Checks {
		if check.ID == "" || seenCheck[check.ID] {
			return nil, fmt.Errorf("validate contract has an empty or duplicate check %q", check.ID)
		}
		seenCheck[check.ID] = true
		if !groups[check.Group] {
			return nil, fmt.Errorf("check %q names unknown group %q", check.ID, check.Group)
		}
		if got := checkGroup[check.ID]; got != "" && got != check.Group {
			return nil, fmt.Errorf("check %q is declared in group %q and %q", check.ID, got, check.Group)
		}
		if len(check.Units) == 0 {
			return nil, fmt.Errorf("check %q declares no object units", check.ID)
		}
		if err := validateUnits(check.Units); err != nil {
			return nil, fmt.Errorf("check %q: %w", check.ID, err)
		}
	}
	for check := range checkGroup {
		if !seenCheck[check] {
			return nil, fmt.Errorf("group lists check %q that has no contract entry", check)
		}
	}
	return &contract, nil
}

var validKinds = map[string]bool{
	"section": true, "item": true, "promise": true, "contradiction": true,
	"file": true, "evidence": true, "rule_section": true, "rule_evidence": true,
	"provider_pair": true, "global": true,
}

var validReads = map[string]bool{
	"section": true, "item": true, "promise": true, "rule": true,
	"provider": true, "evidence": true, "file": true,
	"evidence_section": true,
	"sources":          true, "affects": true, "dependencies": true,
	"items_for_promise": true, "items_sourced_by_section": true,
	"promises_sourced_by_section": true, "file_items": true,
	"evidence_sections_for_items_sourced_by_section": true,
	"all_sections": true, "all_items": true, "all_promises": true,
	"all_rules": true, "all_files": true, "unit": true,
}

var validWhen = map[string]bool{
	"": true, "has_promise": true,
	"has_sources": true, "has_rules": true, "has_files": true,
	"has_evidence_section": true,
}

func validateUnits(units []Unit) error {
	for _, unit := range units {
		if !validKinds[unit.Kind] {
			return fmt.Errorf("unknown unit kind %q", unit.Kind)
		}
		if !validWhen[unit.When] {
			return fmt.Errorf("unknown condition %q", unit.When)
		}
		switch unit.Kind {
		case "global":
			if strings.TrimSpace(unit.Key) == "" {
				return fmt.Errorf("global unit has no key")
			}
		case "section", "item", "promise", "evidence":
			if strings.TrimSpace(unit.Suffix) == "" {
				return fmt.Errorf("%s unit has no suffix", unit.Kind)
			}
		}
		for _, read := range unit.Reads {
			if !validReads[read] {
				return fmt.Errorf("unknown read selector %q", read)
			}
		}
	}
	return nil
}

// BuildCatalog derives the unit validate object catalog from the parsed spec
// graph, the run's extra rule references, and the evidence record, and the contract.
func BuildCatalog(g *Graph, in CatalogInput, contract *Contract) Catalog {
	if in.Unit == "" {
		in.Unit = g.Unit
	}
	b := &builder{
		g:        g,
		in:       in,
		contract: contract,
		seen:     map[string]bool{},
	}
	b.prepare()
	for _, check := range contract.Checks {
		for _, unit := range check.Units {
			b.buildUnit(check.ID, unit)
		}
	}
	sort.Slice(b.keys, func(i, j int) bool { return b.keys[i].ID < b.keys[j].ID })
	return Catalog{Unit: in.Unit, Items: len(g.Items), Promises: len(g.Promises), Sections: len(g.Sections), Keys: b.keys}
}

// GroupsFor returns the coverage groups in contract order.
func GroupsFor(contract *Contract) []Group {
	return append([]Group(nil), contract.Groups...)
}

// GroupForCheck returns the coverage group of a check id.
func GroupForCheck(contract *Contract, check string) (string, bool) {
	for _, group := range contract.Groups {
		for _, c := range group.Checks {
			if c == check {
				return group.ID, true
			}
		}
	}
	return "", false
}

// CheckIDs returns every declared check id in contract order.
func CheckIDs(contract *Contract) []string {
	var out []string
	seen := map[string]bool{}
	for _, group := range contract.Groups {
		for _, check := range group.Checks {
			if !seen[check] {
				seen[check] = true
				out = append(out, check)
			}
		}
	}
	return out
}

type builder struct {
	g                *Graph
	in               CatalogInput
	contract         *Contract
	keys             []Key
	seen             map[string]bool
	rules            []string
	files            []string
	itemsByPromise   map[string][]string
	itemsBySource    map[string][]string
	itemsByFile      map[string][]string
	itemsBySurface   map[string][]string
	promisesBySource map[string][]string
	itemsByID        map[string]Item
	promisesByID     map[string]Promise
	evidenceByItem   map[string]string
}

type buildContext struct {
	section   string
	item      Item
	itemOK    bool
	promise   Promise
	promiseOK bool
	rule      string
	provider  string
	evidence  string
	file      string
	items     []string
}

func (b *builder) prepare() {
	b.itemsByPromise = map[string][]string{}
	b.itemsBySource = map[string][]string{}
	b.itemsByFile = map[string][]string{}
	b.itemsBySurface = map[string][]string{}
	b.promisesBySource = map[string][]string{}
	b.itemsByID = map[string]Item{}
	b.promisesByID = map[string]Promise{}
	b.evidenceByItem = map[string]string{}
	for _, section := range b.in.EvidenceSections {
		b.evidenceByItem[section.Item] = EvidenceSectionRef(section.Item)
	}
	ruleSet := map[string]bool{}
	fileSet := map[string]bool{}
	for _, rule := range b.g.Rules {
		ruleSet[rule] = true
	}
	for _, item := range b.g.Items {
		b.itemsByID[item.ID] = item
		if item.Promise != "" {
			b.itemsByPromise[item.Promise] = append(b.itemsByPromise[item.Promise], item.ID)
		}
		for _, source := range item.Sources {
			b.itemsBySource[source] = append(b.itemsBySource[source], item.ID)
		}
		for _, file := range item.Files {
			fileSet[file] = true
			b.itemsByFile[file] = append(b.itemsByFile[file], item.ID)
		}
		if item.VerificationSurface != "" {
			b.itemsBySurface[item.VerificationSurface] = append(b.itemsBySurface[item.VerificationSurface], item.ID)
		}
		for _, rule := range item.Rules {
			ruleSet[rule] = true
		}
	}
	for _, promise := range b.g.Promises {
		b.promisesByID[promise.ID] = promise
		for _, source := range promise.Sources {
			b.promisesBySource[source] = append(b.promisesBySource[source], promise.ID)
		}
	}
	for _, rule := range b.in.Rules {
		ruleSet[rule] = true
	}
	for rule := range ruleSet {
		b.rules = append(b.rules, rule)
	}
	for file := range fileSet {
		b.files = append(b.files, file)
	}
	sort.Strings(b.rules)
	sort.Strings(b.files)
}

func (b *builder) buildUnit(check string, unit Unit) {
	switch unit.Kind {
	case "section":
		for _, section := range b.g.Sections {
			if section.Level < max(unit.Level, 2) {
				continue
			}
			b.add(check, unit, "section:"+section.Heading+":"+unit.Suffix, b.reads(unit.Reads, buildContext{section: section.Heading}))
		}
	case "item":
		for _, item := range b.g.Items {
			if !b.when(unit.When, buildContext{item: item, itemOK: true}) {
				continue
			}
			b.add(check, unit, "item:"+item.ID+":"+unit.Suffix, b.reads(unit.Reads, buildContext{item: item, itemOK: true}))
		}
	case "promise":
		for _, promise := range b.g.Promises {
			b.add(check, unit, "promise:"+promise.ID+":"+unit.Suffix, b.reads(unit.Reads, buildContext{promise: promise, promiseOK: true}))
		}
	case "evidence":
		if b.in.EvidenceBase != "" {
			b.add(check, unit, "evidence:"+b.in.EvidenceBase+":"+unit.Suffix, b.reads(unit.Reads, buildContext{evidence: b.in.EvidenceBase}))
		}
	case "contradiction":
		for _, pair := range b.contradictionPairs() {
			b.add(check, unit, "contradiction:"+pair[0]+":"+pair[1], []string{itemRef(pair[0]), itemRef(pair[1])})
		}
	case "file":
		for _, file := range b.files {
			b.add(check, unit, "file:"+file, b.reads(unit.Reads, buildContext{file: file}))
		}
	case "rule_section":
		for _, rule := range b.rules {
			for _, section := range b.g.Sections {
				if section.Level < max(unit.Level, 2) {
					continue
				}
				b.add(check, unit, "rule:"+rule+":section:"+section.Heading, b.reads(unit.Reads, buildContext{rule: rule, section: section.Heading}))
			}
		}
	case "rule_evidence":
		if b.in.EvidenceBase != "" {
			for _, rule := range b.rules {
				b.add(check, unit, "rule:"+rule+":evidence", b.reads(unit.Reads, buildContext{rule: rule, evidence: b.in.EvidenceBase}))
			}
		}
	case "provider_pair":
		for _, promise := range b.g.Promises {
			for _, dep := range promise.DependsOn {
				b.add(check, unit, "promise:"+promise.ID+":provider:"+dep, b.reads(unit.Reads, buildContext{promise: promise, promiseOK: true, provider: dep}))
			}
		}
	case "global":
		if !b.when(unit.When, buildContext{}) {
			return
		}
		b.add(check, unit, unit.Key, b.reads(unit.Reads, buildContext{evidence: b.in.EvidenceBase}))
	}
}

func (b *builder) when(condition string, ctx buildContext) bool {
	switch condition {
	case "":
		return true
	case "has_promise":
		return ctx.itemOK && ctx.item.Promise != ""
	case "has_sources":
		return (ctx.itemOK && len(ctx.item.Sources) > 0) || (ctx.promiseOK && len(ctx.promise.Sources) > 0)
	case "has_evidence_section":
		return ctx.itemOK && b.evidenceByItem[ctx.item.ID] != ""
	case "has_rules":
		return len(b.rules) > 0
	case "has_files":
		return len(b.files) > 0
	}
	return false
}

func (b *builder) add(check string, unit Unit, id string, reads []string) {
	if id == "" || b.seen[id] {
		return
	}
	reads = dedupeStrings(reads)
	if len(reads) == 0 {
		return
	}
	b.seen[id] = true
	b.keys = append(b.keys, Key{ID: id, Family: familyFor(unit.Kind), Check: check, Reads: reads})
}

func (b *builder) reads(names []string, ctx buildContext) []string {
	var out []string
	for _, name := range names {
		switch name {
		case "section":
			if ctx.section != "" {
				out = append(out, sectionRef(ctx.section))
			}
		case "item":
			if len(ctx.items) > 0 {
				for _, item := range ctx.items {
					out = append(out, itemRef(item))
				}
			} else if ctx.itemOK {
				out = append(out, itemRef(ctx.item.ID))
			}
		case "promise":
			if ctx.promiseOK {
				out = append(out, promiseRef(ctx.promise.ID))
			} else if ctx.itemOK && ctx.item.Promise != "" {
				out = append(out, promiseRef(ctx.item.Promise))
			}
		case "rule":
			if ctx.rule != "" {
				out = append(out, ruleRef(ctx.rule))
			}
		case "provider":
			if ctx.provider != "" {
				out = append(out, providerRef(ctx.provider))
			}
		case "evidence":
			if ctx.evidence != "" {
				out = append(out, "evidence:"+ctx.evidence)
			}
		case "evidence_section":
			if ctx.itemOK {
				if ref, ok := b.evidenceByItem[ctx.item.ID]; ok {
					out = append(out, ref)
				}
			}
		case "evidence_sections_for_items_sourced_by_section":
			if ctx.section != "" {
				for _, item := range b.itemsBySource[ctx.section] {
					if ref, ok := b.evidenceByItem[item]; ok {
						out = append(out, ref)
					}
				}
			}
		case "file":
			if ctx.file != "" {
				out = append(out, fileRef(ctx.file))
			}
		case "sources":
			switch {
			case ctx.promiseOK:
				for _, source := range ctx.promise.Sources {
					out = append(out, sectionRef(source))
				}
			case ctx.itemOK:
				for _, source := range ctx.item.Sources {
					out = append(out, sectionRef(source))
				}
			}
		case "affects":
			if ctx.itemOK {
				for _, file := range ctx.item.Files {
					out = append(out, fileRef(file))
				}
				for _, rule := range ctx.item.Rules {
					out = append(out, ruleRef(rule))
				}
			}
		case "dependencies":
			if ctx.promiseOK {
				for _, dep := range ctx.promise.DependsOn {
					out = append(out, providerRef(dep))
				}
			}
		case "items_for_promise":
			if ctx.promiseOK {
				for _, item := range b.itemsByPromise[ctx.promise.ID] {
					out = append(out, itemRef(item))
				}
			}
		case "items_sourced_by_section":
			if ctx.section != "" {
				for _, item := range b.itemsBySource[ctx.section] {
					out = append(out, itemRef(item))
				}
			}
		case "promises_sourced_by_section":
			if ctx.section != "" {
				for _, promise := range b.promisesBySource[ctx.section] {
					out = append(out, promiseRef(promise))
				}
			}
		case "file_items":
			if ctx.file != "" {
				for _, item := range b.itemsByFile[ctx.file] {
					out = append(out, itemRef(item))
				}
			}
		case "all_sections":
			for _, section := range b.g.Sections {
				out = append(out, sectionRef(section.Heading))
			}
		case "all_items":
			for _, item := range b.g.Items {
				out = append(out, itemRef(item.ID))
			}
		case "all_promises":
			for _, promise := range b.g.Promises {
				out = append(out, promiseRef(promise.ID))
			}
		case "all_rules":
			for _, rule := range b.rules {
				out = append(out, ruleRef(rule))
			}
		case "all_files":
			for _, file := range b.files {
				out = append(out, fileRef(file))
			}
		case "unit":
			out = append(out, unitRef(b.in.Unit))
		}
	}
	return out
}

func (b *builder) contradictionPairs() [][2]string {
	pairs := map[[2]string]bool{}
	addGroups := func(groups map[string][]string) {
		for _, ids := range groups {
			ids = append([]string(nil), ids...)
			sort.Strings(ids)
			for i := 0; i < len(ids); i++ {
				for j := i + 1; j < len(ids); j++ {
					pairs[[2]string{ids[i], ids[j]}] = true
				}
			}
		}
	}
	addGroups(b.itemsByPromise)
	addGroups(b.itemsBySource)
	addGroups(b.itemsBySurface)
	addGroups(b.itemsByFile)
	var out [][2]string
	for pair := range pairs {
		out = append(out, pair)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i][0] != out[j][0] {
			return out[i][0] < out[j][0]
		}
		return out[i][1] < out[j][1]
	})
	return out
}

func familyFor(kind string) Family {
	switch kind {
	case "section", "evidence":
		return FamilySection
	case "item", "contradiction":
		return FamilyItem
	case "promise", "provider_pair":
		return FamilyPromise
	case "rule_section", "rule_evidence", "file":
		return FamilyExternal
	default:
		return FamilyGlobal
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
