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

// Surface field identifiers used by the association view.
const (
	SurfaceFieldImplementation = "implementation_surface"
	SurfaceFieldAffects        = "affects.files"
)

// SurfaceRef identifies one code-surface declaration: the unit that declared
// it, the field it was declared in, the canonical repository-relative file it
// resolves to, and — when the declaration was a directory — the directory that
// expanded to it.
type SurfaceRef struct {
	Unit  string `json:"unit"`
	Field string `json:"field"`
	Path  string `json:"path"`
	Via   string `json:"via,omitempty"`
	Layer string `json:"layer"`
}

// SharedSurfaceFile is one code file declared by two or more current-layer units.
type SharedSurfaceFile struct {
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

// SurfaceUnitAudit is one unit's deduplicated effective code surface.
type SurfaceUnitAudit struct {
	Unit  string       `json:"unit"`
	Layer string       `json:"layer"`
	Files []SurfaceRef `json:"files"`
}

// SurfaceAuditReport is the repo-wide result of SurfaceAudit.
type SurfaceAuditReport struct {
	Units       []SurfaceUnitAudit  `json:"units"`
	SharedFiles []SharedSurfaceFile `json:"shared_files"`
	Directories []SurfaceDirectory  `json:"directories"`
}

// SurfaceAudit derives file associations from current and stable declarations.
// Sharing a file creates no behavioral dependency. Invalid declarations remain
// the responsibility of the anchor checks.
func SurfaceAudit(repoRoot string) (*SurfaceAuditReport, error) {
	graph, err := unitgraph.Build(repoRoot, "all")
	if err != nil {
		return nil, fmt.Errorf("build unit graph: %w", err)
	}

	nodes := graph.Nodes()
	stable, err := unitgraph.Build(repoRoot, "stable")
	if err != nil {
		return nil, err
	}
	for _, n := range stable.Nodes() {
		duplicate := false
		for _, current := range nodes {
			if current.Name == n.Name && current.Layer == "stable" {
				duplicate = true
			}
		}
		if !duplicate {
			nodes = append(nodes, n)
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Name != nodes[j].Name {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].Layer < nodes[j].Layer
	})

	report := &SurfaceAuditReport{}
	decls := map[string][]SurfaceRef{}

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
		for i := range files {
			files[i].Layer = node.Layer
		}
		report.Units = append(report.Units, SurfaceUnitAudit{Unit: node.Name, Layer: node.Layer, Files: files})
		report.Directories = append(report.Directories, collector.directories...)

		for _, f := range files {
			decls[f.Path] = append(decls[f.Path], f)
		}
	}

	for path, refs := range decls {
		if distinctDeclUnits(refs) < 2 {
			continue
		}
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].Unit != refs[j].Unit {
				return refs[i].Unit < refs[j].Unit
			}
			if refs[i].Layer != refs[j].Layer {
				return refs[i].Layer < refs[j].Layer
			}
			return refs[i].Field < refs[j].Field
		})
		report.SharedFiles = append(report.SharedFiles, SharedSurfaceFile{Path: path, Decls: refs})
	}
	sort.Slice(report.SharedFiles, func(i, j int) bool { return report.SharedFiles[i].Path < report.SharedFiles[j].Path })

	sort.Slice(report.Directories, func(i, j int) bool {
		a, b := report.Directories[i], report.Directories[j]
		if a.Unit != b.Unit {
			return a.Unit < b.Unit
		}
		return a.Path < b.Path
	})

	return report, nil
}

// surfaceCollector deduplicates associations in one unit layer.
// Prefer implementation_surface when both declaration fields name a file.
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

// checkSurfaceAssociations provides declaration evidence for semantic Check 9.
func checkSurfaceAssociations(repoRoot, unitName string) CheckResult {
	const name = "Surface associations"
	if _, err := SurfaceAudit(repoRoot); err != nil {
		return CheckResult{Name: name, Status: Fail, Details: err.Error()}
	}
	return CheckResult{Name: name, Status: Pass, Details: "file associations are valid; shared agreements require semantic Check 9"}
}

// FormatSurfaceAudit displays associations, including confirmed stable designs.
func FormatSurfaceAudit(report *SurfaceAuditReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Surface associations — %d unit layer(s), %d shared file(s)\n", len(report.Units), len(report.SharedFiles))
	for _, unit := range report.Units {
		fmt.Fprintf(&b, "\n%s (%s):\n", unit.Unit, unit.Layer)
		for _, f := range unit.Files {
			fmt.Fprintf(&b, "  %s (%s)\n", f.Path, f.Field)
		}
	}
	return b.String()
}
