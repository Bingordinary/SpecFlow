package specvalidation

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repofiles"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/repopath"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/unitgraph"
)

// Surface field identifiers used by the ownership audit.
const (
	SurfaceFieldImplementation = "implementation_surface"
	SurfaceFieldAffects        = "affects.files"
)

// surfaceProblemLimit caps how many violations one check-line details string
// lists; the full set is available through `specflowctl surfaces`.
const surfaceProblemLimit = 10

const surfaceOwnershipGuidance = "a code file may have only one owner: keep it in the owning unit and reference that unit via unit_refs/affects.dependencies; extract shared constraint content into a rule (rule_refs); carry shared code by a unit (extract a new unit when no existing unit owns it) — framework/spec_writing_guide.md §14.4"

// SurfaceRef identifies one code-surface declaration: the unit that declared
// it, the field it was declared in, the canonical repository-relative file it
// resolves to, and — when the declaration was a directory — the directory that
// expanded to it.
type SurfaceRef struct {
	Unit  string `json:"unit"`
	Field string `json:"field"`
	Path  string `json:"path"`
	Via   string `json:"via,omitempty"`
}

// SurfaceOverlap is one code file declared by two or more current-layer units.
type SurfaceOverlap struct {
	Path  string       `json:"path"`
	Decls []SurfaceRef `json:"declarations"`
}

// SurfaceDirectory is one directory-valued surface declaration.
type SurfaceDirectory struct {
	Unit      string `json:"unit"`
	Field     string `json:"field"`
	Path      string `json:"path"`
	FileCount int    `json:"file_count"`
}

// SurfaceOutOfBounds is an affects.files entry that points at another unit's
// implementation_surface file. DepDeclared reports whether that owner is
// already listed in the declaring unit's unit_refs.
type SurfaceOutOfBounds struct {
	Unit        string `json:"unit"`
	Entry       string `json:"entry"`
	Owner       string `json:"owner"`
	DepDeclared bool   `json:"dep_declared"`
}

// SurfaceUnitAudit is one unit's deduplicated effective code surface.
type SurfaceUnitAudit struct {
	Unit  string       `json:"unit"`
	Layer string       `json:"layer"`
	Files []SurfaceRef `json:"files"`
}

// SurfaceAuditReport is the repo-wide result of SurfaceAudit.
type SurfaceAuditReport struct {
	Units       []SurfaceUnitAudit   `json:"units"`
	Overlaps    []SurfaceOverlap     `json:"overlaps"`
	Directories []SurfaceDirectory   `json:"directories"`
	OutOfBounds []SurfaceOutOfBounds `json:"out_of_bounds"`
}

// SurfaceAudit scans every current-layer unit's declared code surface
// (implementation_surface and affects.files; directories expand to their
// repository-content files) and reports cross-unit ownership violations:
// files declared by more than one unit, directory-valued declarations, and
// affects.files entries that point at another unit's implementation_surface
// file. Spec paths (docs/specs/**) are spec references, not code surfaces, and
// do not participate. Unresolvable declarations are skipped — the anchor check
// reports them.
func SurfaceAudit(repoRoot string) (*SurfaceAuditReport, error) {
	graph, err := unitgraph.Build(repoRoot, "all")
	if err != nil {
		return nil, fmt.Errorf("build unit graph: %w", err)
	}

	nodes := graph.Nodes()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })

	report := &SurfaceAuditReport{}
	decls := map[string][]SurfaceRef{}
	implByUnit := map[string]map[string]bool{}
	unitRefs := map[string]map[string]bool{}

	for _, node := range nodes {
		ref := specpaths.CandidateUnitSpecFileRef(node.Name)
		if node.Layer == "stable" {
			ref = specpaths.StableUnitSpecFileRef(node.Name)
		}
		data, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(ref)))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", ref, err)
		}
		content := string(data)

		collector := newSurfaceCollector(node.Name)
		for _, value := range ExtractImplementationSurfaces(content) {
			collector.addDeclaration(repoRoot, SurfaceFieldImplementation, value)
		}
		for _, value := range ExtractAffectsFiles(content) {
			collector.addDeclaration(repoRoot, SurfaceFieldAffects, value)
		}
		files := collector.orderedRefs()
		report.Units = append(report.Units, SurfaceUnitAudit{Unit: node.Name, Layer: node.Layer, Files: files})
		report.Directories = append(report.Directories, collector.directories...)

		for _, f := range files {
			decls[f.Path] = append(decls[f.Path], f)
			if f.Field == SurfaceFieldImplementation {
				if implByUnit[node.Name] == nil {
					implByUnit[node.Name] = map[string]bool{}
				}
				implByUnit[node.Name][f.Path] = true
			}
		}

		refs := map[string]bool{}
		for _, ref := range node.UnitRefs {
			refs[ref] = true
		}
		unitRefs[node.Name] = refs
	}

	for path, refs := range decls {
		if distinctDeclUnits(refs) < 2 {
			continue
		}
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].Unit != refs[j].Unit {
				return refs[i].Unit < refs[j].Unit
			}
			return refs[i].Field < refs[j].Field
		})
		report.Overlaps = append(report.Overlaps, SurfaceOverlap{Path: path, Decls: refs})
	}
	sort.Slice(report.Overlaps, func(i, j int) bool { return report.Overlaps[i].Path < report.Overlaps[j].Path })

	for _, unit := range report.Units {
		for _, f := range unit.Files {
			if f.Field != SurfaceFieldAffects {
				continue
			}
			for owner, files := range implByUnit {
				if owner == unit.Unit || !files[f.Path] {
					continue
				}
				report.OutOfBounds = append(report.OutOfBounds, SurfaceOutOfBounds{
					Unit:        unit.Unit,
					Entry:       f.Path,
					Owner:       owner,
					DepDeclared: unitRefs[unit.Unit][owner],
				})
			}
		}
	}
	sort.Slice(report.OutOfBounds, func(i, j int) bool {
		a, b := report.OutOfBounds[i], report.OutOfBounds[j]
		if a.Unit != b.Unit {
			return a.Unit < b.Unit
		}
		if a.Entry != b.Entry {
			return a.Entry < b.Entry
		}
		return a.Owner < b.Owner
	})
	sort.Slice(report.Directories, func(i, j int) bool {
		a, b := report.Directories[i], report.Directories[j]
		if a.Unit != b.Unit {
			return a.Unit < b.Unit
		}
		return a.Path < b.Path
	})

	return report, nil
}

// surfaceCollector accumulates one unit's declarations, deduplicating files a
// unit declares more than once (e.g. in both fields or through overlapping
// directories). The implementation_surface attribution is preferred because
// it names the file as the unit's own implementation, which is the fact the
// out-of-bounds comparison needs.
type surfaceCollector struct {
	unit        string
	refs        map[string]SurfaceRef
	order       []string
	directories []SurfaceDirectory
}

func newSurfaceCollector(unit string) *surfaceCollector {
	return &surfaceCollector{unit: unit, refs: map[string]SurfaceRef{}}
}

// addDeclaration resolves one declared value. Unresolvable or spec-document
// values are skipped: the anchor check owns those failures, and spec paths are
// references rather than code surfaces.
func (c *surfaceCollector) addDeclaration(repoRoot, field, value string) {
	value = strings.TrimSpace(value)
	if value == "" || value == SurfacePending {
		return
	}
	canonical, err := repopath.Canonical(repoRoot, value)
	if err != nil || strings.HasPrefix(canonical, "docs/specs/") {
		return
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(canonical))
	info, err := os.Stat(abs)
	if err != nil {
		return
	}
	if !info.IsDir() {
		c.add(canonical, field, "")
		return
	}
	files, err := repofiles.ExpandDir(repoRoot, canonical)
	if err != nil {
		return
	}
	c.directories = append(c.directories, SurfaceDirectory{
		Unit: c.unit, Field: field, Path: canonical, FileCount: len(files),
	})
	for _, f := range files {
		c.add(f.Path, field, canonical)
	}
}

func (c *surfaceCollector) add(path, field, via string) {
	existing, ok := c.refs[path]
	if !ok {
		c.refs[path] = SurfaceRef{Unit: c.unit, Field: field, Path: path, Via: via}
		c.order = append(c.order, path)
		return
	}
	if existing.Field == SurfaceFieldAffects && field == SurfaceFieldImplementation {
		existing.Field = field
		existing.Via = via
		c.refs[path] = existing
	}
}

func (c *surfaceCollector) orderedRefs() []SurfaceRef {
	out := make([]SurfaceRef, 0, len(c.order))
	for _, path := range c.order {
		out = append(out, c.refs[path])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func distinctDeclUnits(refs []SurfaceRef) int {
	seen := map[string]bool{}
	for _, ref := range refs {
		seen[ref.Unit] = true
	}
	return len(seen)
}

func formatSurfaceDecl(ref SurfaceRef) string {
	if ref.Via != "" {
		return fmt.Sprintf("%s (%s via %s)", ref.Unit, ref.Field, ref.Via)
	}
	return fmt.Sprintf("%s (%s)", ref.Unit, ref.Field)
}

// checkSurfaceOwnership is the mechanical surface-ownership check: it reports
// every overlap and out-of-bounds affects entry that involves this unit. The
// semantic resolutions (who owns the file, which parts become a rule, whether
// shared code becomes a unit) are judged by the agent validate Check 9.
func checkSurfaceOwnership(repoRoot, unitName string) CheckResult {
	const name = "Surface ownership"

	data, err := os.ReadFile(specPath(repoRoot, unitName))
	if err != nil {
		return CheckResult{Name: name, Status: Fail, Details: fmt.Sprintf("cannot read candidate spec: %v", err)}
	}
	fm := specpaths.ReadFrontmatterStringMap(string(data))
	if strings.TrimSpace(fm["status"]) == "retired" {
		return CheckResult{Name: name, Status: Pass, Details: "spec is marked retired — surface ownership check skipped"}
	}

	report, err := SurfaceAudit(repoRoot)
	if err != nil {
		return CheckResult{Name: name, Status: Fail, Details: fmt.Sprintf("cannot audit unit surfaces: %v", err)}
	}

	var problems []string
	for _, overlap := range report.Overlaps {
		involvesUnit := false
		var others []string
		for _, decl := range overlap.Decls {
			if decl.Unit == unitName {
				involvesUnit = true
				continue
			}
			others = append(others, formatSurfaceDecl(decl))
		}
		if !involvesUnit {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s is also declared by %s", overlap.Path, strings.Join(others, ", ")))
	}
	for _, outOfBounds := range report.OutOfBounds {
		if outOfBounds.Unit != unitName {
			continue
		}
		dep := "unit_refs missing"
		if outOfBounds.DepDeclared {
			dep = "unit_refs declared"
		}
		problems = append(problems, fmt.Sprintf("%s (affects.files) belongs to unit %s's implementation_surface (%s)", outOfBounds.Entry, outOfBounds.Owner, dep))
	}

	if len(problems) == 0 {
		return CheckResult{Name: name, Status: Pass, Details: "no cross-unit surface overlap"}
	}

	details := strings.Join(problems[:min(len(problems), surfaceProblemLimit)], "; ")
	if len(problems) > surfaceProblemLimit {
		details += fmt.Sprintf("; and %d more (run `specflowctl surfaces` for the full report)", len(problems)-surfaceProblemLimit)
	}
	return CheckResult{Name: name, Status: Fail, Details: details + ". " + surfaceOwnershipGuidance}
}

// FormatSurfaceAudit renders the audit as a human-readable report.
func FormatSurfaceAudit(report *SurfaceAuditReport) string {
	var b strings.Builder

	files := 0
	for _, unit := range report.Units {
		files += len(unit.Files)
	}
	fmt.Fprintf(&b, "Surface ownership audit — %d unit(s), %d file(s), %d overlap(s), %d directory declaration(s), %d out-of-bounds entry(ies)\n",
		len(report.Units), files, len(report.Overlaps), len(report.Directories), len(report.OutOfBounds))

	if len(report.Units) > 0 {
		b.WriteString("\nUnits:\n")
		for _, unit := range report.Units {
			fmt.Fprintf(&b, "  %s (%s): %d file(s)\n", unit.Unit, unit.Layer, len(unit.Files))
		}
	}
	if len(report.Directories) > 0 {
		b.WriteString("\nDirectory declarations:\n")
		for _, dir := range report.Directories {
			fmt.Fprintf(&b, "  %s %s: %s -> %d file(s)\n", dir.Unit, dir.Field, dir.Path, dir.FileCount)
		}
	}
	if len(report.Overlaps) > 0 {
		b.WriteString("\nOverlaps:\n")
		for _, overlap := range report.Overlaps {
			fmt.Fprintf(&b, "  %s\n", overlap.Path)
			for _, decl := range overlap.Decls {
				fmt.Fprintf(&b, "    - %s\n", formatSurfaceDecl(decl))
			}
		}
	}
	if len(report.OutOfBounds) > 0 {
		b.WriteString("\nOut-of-bounds affects.files entries:\n")
		for _, entry := range report.OutOfBounds {
			dep := "unit_refs missing"
			if entry.DepDeclared {
				dep = "unit_refs declared"
			}
			fmt.Fprintf(&b, "  %s affects.files %s -> owned by %s (%s)\n", entry.Unit, entry.Entry, entry.Owner, dep)
		}
	}
	if len(report.Overlaps) == 0 && len(report.OutOfBounds) == 0 {
		b.WriteString("\nNo cross-unit surface overlap.\n")
	}
	return b.String()
}
