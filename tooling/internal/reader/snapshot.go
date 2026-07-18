package reader

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type markdownDoc struct {
	RelPath     string
	Title       string
	Frontmatter frontmatter
	Text        string
}

var sharedRefPattern = regexp.MustCompile("`?([cs]_[gb]_rule_[A-Za-z0-9_]+@[0-9]+\\.[0-9]+\\.[0-9]+)`?")
var unitRefPattern = regexp.MustCompile("`?s_unit_([A-Za-z0-9_]+)@[0-9]+\\.[0-9]+\\.[0-9]+`?")
var appendixUnitPattern = regexp.MustCompile(`^[cs]_unit_([A-Za-z0-9_-]+)_.*\.md$`)

func BuildSnapshot(repoRoot string) Snapshot {
	repoRoot, _ = filepath.Abs(repoRoot)
	docs, docDiagnostics := loadTruthDocs(repoRoot)

	docByPath := map[string]markdownDoc{}
	for _, doc := range docs {
		docByPath[doc.RelPath] = doc
	}

	snapshot := Snapshot{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Project: ProjectInfo{
			RepoRoot:         repoRoot,
			RuleBaselineFile: "docs/specs/rules/stable/s_g_rule_repository_baseline.md",
		},
		Diagnostics: docDiagnostics,
	}

	builder := newGraphBuilder()
	sourceSet := map[string]SourceRef{}
	addSource := func(ref SourceRef) {
		if ref.Path == "" {
			return
		}
		key := ref.Path
		if existing, ok := sourceSet[key]; !ok || existing.Line == 0 {
			sourceSet[key] = ref
		}
	}
	addSource(SourceRef{Path: "docs/specs/rules/stable/s_g_rule_repository_baseline.md", Label: "Global Rules"})

	for _, doc := range docs {
		addSource(SourceRef{Path: doc.RelPath, Label: doc.Title})
	}

	builder.addNode(GraphNode{ID: "rule:baseline", Kind: "rule", Label: "Global Rules", Group: "rule", Source: ptr(SourceRef{Path: "docs/specs/rules/stable/s_g_rule_repository_baseline.md"})})

	// Phase 1: Build objects from filesystem spec files
	// First collect all unit and rule objects from their spec file frontmatter
	seenIDs := map[string]bool{}
	docSets := map[string]*struct {
		Kind   string
		Doc    markdownDoc
		Refs   []struct {
			title string
			path  string
			isCand bool
			isStable bool
			doc    markdownDoc
		}
	}{}
	for _, doc := range docs {
		if isAppendixPath(doc.RelPath) {
			continue
		}
		id := ""
		kind := ""
		switch {
		case strings.Contains(doc.RelPath, "/units/"):
			kind = "unit"
			id = strings.TrimSpace(doc.Frontmatter.Scalars["id"])
		case strings.Contains(doc.RelPath, "/rules/"):
			kind = "rule"
			id = strings.TrimSpace(doc.Frontmatter.Scalars["rule_id"])
		}
		if kind == "" || id == "" {
			continue
		}
		isCand := strings.Contains(doc.RelPath, "/candidate/")
		isStable := strings.Contains(doc.RelPath, "/stable/")
		key := kind + ":" + id
		if _, ok := docSets[key]; !ok {
			docSets[key] = &struct {
				Kind   string
				Doc    markdownDoc
				Refs   []struct {
					title string
					path  string
					isCand bool
					isStable bool
					doc    markdownDoc
				}
			}{Kind: kind, Doc: doc}
		}
		docSets[key].Refs = append(docSets[key].Refs, struct {
			title string
			path  string
			isCand bool
			isStable bool
			doc    markdownDoc
		}{
			title:    doc.Title,
			path:     doc.RelPath,
			isCand:   isCand,
			isStable: isStable,
			doc:      doc,
		})
	}
	for key, ds := range docSets {
		parts := strings.SplitN(key, ":", 2)
		kind := parts[0]
		id := parts[1]
		seenIDs[key] = true

		object := ObjectView{
			ID:    id,
			Kind:  kind,
			Label: id,
		}
		for _, ref := range ds.Refs {
			if ref.isCand {
				object.HasCandidate = true
			}
			if ref.isStable {
				object.HasStable = true
			}
			object.TruthPaths = appendSourceUnique(object.TruthPaths, SourceRef{Path: ref.path, Label: ref.title})
			object.Sources = appendSourceUnique(object.Sources, SourceRef{Path: ref.path, Label: ref.title})
		}
		if object.HasCandidate {
			object.Layer = "candidate"
		} else if object.HasStable {
			object.Layer = "stable"
		}
		object.Version = ds.Doc.Frontmatter.Scalars["version"]
		object.RuleRefs = extractRuleIDsFromFrontmatter(ds.Doc.Frontmatter)
		object.UnitRefs = extractUnitIDsFromFrontmatter(ds.Doc.Frontmatter)
		sort.Strings(object.RuleRefs)
		sort.Strings(object.UnitRefs)

		snapshot.Objects = append(snapshot.Objects, object)
		if kind == "unit" {
			snapshot.Project.UnitCount++
		} else if kind == "rule" {
			snapshot.Project.RuleCount++
		}
	}

	// Phase 3b: Associate appendix files with their parent objects
	for _, doc := range docs {
		if !isAppendixPath(doc.RelPath) {
			continue
		}
		kind := ""
		id := ""
		switch {
		case strings.Contains(doc.RelPath, "/units/"):
			kind = "unit"
			matches := appendixUnitPattern.FindStringSubmatch(filepath.Base(doc.RelPath))
			if len(matches) > 1 {
				id = matches[1]
			}
		}
		if kind == "" || id == "" {
			continue
		}
		key := kind + ":" + id
		if !seenIDs[key] {
			seenIDs[key] = true
			object := ObjectView{
				ID:    id,
				Kind:  kind,
				Label: id,
				HasCandidate: strings.Contains(doc.RelPath, "/candidate/"),
				HasStable:    strings.Contains(doc.RelPath, "/stable/"),
				Version:      doc.Frontmatter.Scalars["version"],
				RuleRefs:     extractRuleIDsFromFrontmatter(doc.Frontmatter),
				UnitRefs:     extractUnitIDsFromFrontmatter(doc.Frontmatter),
			}
			sort.Strings(object.RuleRefs)
			sort.Strings(object.UnitRefs)
			snapshot.Objects = append(snapshot.Objects, object)
		}
		for i := range snapshot.Objects {
			if snapshot.Objects[i].Kind == kind && snapshot.Objects[i].ID == id {
				snapshot.Objects[i].TruthPaths = appendSourceUnique(snapshot.Objects[i].TruthPaths, SourceRef{Path: doc.RelPath, Label: doc.Title})
				snapshot.Objects[i].Sources = appendSourceUnique(snapshot.Objects[i].Sources, SourceRef{Path: doc.RelPath, Label: doc.Title})
				if snapshot.Objects[i].Version == "" {
					snapshot.Objects[i].Version = doc.Frontmatter.Scalars["version"]
				}
				snapshot.Objects[i].RuleRefs = appendUnique(snapshot.Objects[i].RuleRefs, extractRuleIDsFromFrontmatter(doc.Frontmatter)...)
				snapshot.Objects[i].UnitRefs = appendUnique(snapshot.Objects[i].UnitRefs, extractUnitIDsFromFrontmatter(doc.Frontmatter)...)
				sort.Strings(snapshot.Objects[i].RuleRefs)
				sort.Strings(snapshot.Objects[i].UnitRefs)
				break
			}
		}
		addSource(SourceRef{Path: doc.RelPath, Label: doc.Title})
	}

	// Phase 4: Build graph nodes and edges
	boundMap := boundObjectsByRuleID(snapshot.Objects)
	for _, object := range snapshot.Objects {
		nodeID := object.Kind + ":" + object.ID
		group := object.Kind
		builder.addNode(GraphNode{ID: nodeID, Kind: object.Kind, Label: object.ID, Group: group, Source: firstSource(object.Sources)})

		for _, truth := range object.TruthPaths {
			addSource(truth)
			fileNode := "file:" + truth.Path
			builder.addNode(GraphNode{ID: fileNode, Kind: "truth_file", Label: compactTruthFileLabel(filepath.Base(truth.Path)), Group: "truth", Source: ptr(SourceRef{Path: truth.Path})})
			builder.addEdge(GraphEdge{ID: nodeID + "->" + fileNode, From: nodeID, To: fileNode, Kind: "described_by", Label: "described by", Source: ptr(truth)})
		}
		for _, impl := range object.ImplementationPaths {
			pathNode := "path:" + impl.Path
			builder.addNode(GraphNode{ID: pathNode, Kind: "implementation_path", Label: impl.Path, Group: "implementation", Source: ptr(impl)})
			builder.addEdge(GraphEdge{ID: nodeID + "->" + pathNode, From: nodeID, To: pathNode, Kind: "owns_path", Label: "owns path", Source: ptr(impl)})
		}
		for _, ruleID := range object.RuleRefs {
			sharedNode := "shared:" + ruleID
			builder.addNode(GraphNode{ID: sharedNode, Kind: "rule", Label: ruleID, Group: "shared"})
			builder.addEdge(GraphEdge{ID: nodeID + "->" + sharedNode, From: nodeID, To: sharedNode, Kind: "uses_shared", Label: "uses shared", Source: firstSource(object.Sources)})
		}
		for _, unitID := range object.UnitRefs {
			dependencyNode := "unit:" + unitID
			builder.addNode(GraphNode{ID: dependencyNode, Kind: "unit", Label: unitID, Group: "unit"})
			builder.addEdge(GraphEdge{ID: nodeID + "->" + dependencyNode, From: nodeID, To: dependencyNode, Kind: "depends_on", Label: "depends on", Source: firstSource(object.Sources)})
		}
		if object.Kind == "rule" {
			for _, unitKey := range boundMap[object.ID] {
				builder.addEdge(GraphEdge{ID: nodeID + "->" + unitKey, From: nodeID, To: unitKey, Kind: "bound_to", Label: "bound to", Source: firstSource(object.Sources)})
			}
		}
	}

	snapshot.Project.TruthFileCount = len(docs)
	snapshot.Registry = buildRegistryItems(docs, snapshot.Objects)
	snapshot.Nodes = builder.nodes()
	snapshot.Edges = builder.edges()
	snapshot.Sources = sortedSources(sourceSet)

	sortObjects(snapshot.Objects)
	normalizeSnapshotSlices(&snapshot)
	return snapshot
}

func docExists(docs map[string]markdownDoc, path string) bool {
	_, ok := docs[path]
	return ok
}

func loadTruthDocs(repoRoot string) ([]markdownDoc, []Diagnostic) {
	root := filepath.Join(repoRoot, "docs/specs")
	diagnostics := []Diagnostic{}
	docs := []markdownDoc{}
	if _, err := os.Stat(root); err != nil {
		return docs, []Diagnostic{{Severity: "error", Message: "cannot read docs/specs: " + err.Error(), Source: &SourceRef{Path: "docs/specs"}}}
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		data, err := os.ReadFile(path)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{Severity: "error", Message: "cannot read " + rel + ": " + err.Error(), Source: &SourceRef{Path: rel}})
			return nil
		}
		text := string(data)
		docs = append(docs, markdownDoc{
			RelPath:     rel,
			Title:       firstTitle(text, rel),
			Frontmatter: parseFrontmatter(text),
			Text:        text,
		})
		return nil
	})
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{Severity: "error", Message: "cannot walk docs/specs: " + err.Error(), Source: &SourceRef{Path: "docs/specs"}})
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].RelPath < docs[j].RelPath })
	return docs, diagnostics
}

func firstTitle(text, fallback string) string {
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return filepath.Base(fallback)
}

func compactTruthFileLabel(filename string) string {
	base := strings.TrimSuffix(filename, ".md")
	switch {
	case strings.HasPrefix(base, "c_unit_"):
		return strings.ReplaceAll(strings.TrimPrefix(base, "c_unit_"), "_", " ") + " (candidate)"
	case strings.HasPrefix(base, "s_unit_"):
		return strings.ReplaceAll(strings.TrimPrefix(base, "s_unit_"), "_", " ") + " (stable)"
	case strings.HasPrefix(base, "c_b_rule_"):
		return "shared " + strings.ReplaceAll(strings.TrimPrefix(base, "c_b_rule_"), "_", " ") + " (candidate)"
	case strings.HasPrefix(base, "s_b_rule_"):
		return "shared " + strings.ReplaceAll(strings.TrimPrefix(base, "s_b_rule_"), "_", " ") + " (stable)"
	default:
		return strings.ReplaceAll(base, "_", " ")
	}
}

func extractRuleIDs(text string) []string {
	matches := sharedRefPattern.FindAllStringSubmatch(text, -1)
	ids := []string{}
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		id := sharedRefToID(match[1])
		if id != "" {
			ids = appendUnique(ids, id)
		}
	}
	return ids
}

func extractRuleIDsFromFrontmatter(fm frontmatter) []string {
	ids := []string{}
	for _, ref := range frontmatterRefs(fm, "rule_refs") {
		id := sharedRefToID(ref)
		if id != "" {
			ids = appendUnique(ids, id)
		}
	}
	return ids
}

func extractUnitIDsFromFrontmatter(fm frontmatter) []string {
	ids := []string{}
	for _, ref := range frontmatterRefs(fm, "unit_refs") {
		matches := unitRefPattern.FindStringSubmatch(ref)
		if len(matches) != 2 {
			continue
		}
		ids = appendUnique(ids, strings.TrimSpace(matches[1]))
	}
	return ids
}

func frontmatterRefs(fm frontmatter, key string) []string {
	values := []string{}
	for _, value := range fm.Lists[key] {
		value = strings.TrimSpace(value)
		if value != "" && value != "none" {
			values = append(values, value)
		}
	}
	scalar := strings.TrimSpace(fm.Scalars[key])
	if scalar != "" && scalar != "none" {
		for _, value := range strings.Split(scalar, ",") {
			value = strings.TrimSpace(value)
			if value != "" && value != "none" {
				values = append(values, value)
			}
		}
	}
	return values
}

func extractUnitIDs(text string) []string {
	matches := unitRefPattern.FindAllStringSubmatch(text, -1)
	ids := []string{}
	for _, match := range matches {
		if len(match) != 2 {
			continue
		}
		ids = appendUnique(ids, strings.TrimSpace(match[1]))
	}
	return ids
}

func sharedRefToID(ref string) string {
	ref = strings.TrimSpace(ref)
	if before, _, ok := strings.Cut(ref, "@"); ok {
		ref = before
	}
	ref = strings.TrimPrefix(ref, "c_")
	ref = strings.TrimPrefix(ref, "s_")
	return ref
}

func boundObjectsByRuleID(objects []ObjectView) map[string][]string {
	result := map[string][]string{}
	for _, object := range objects {
		if object.Kind != "unit" {
			continue
		}
		for _, ruleID := range object.RuleRefs {
			if ruleID == "" {
				continue
			}
			result[ruleID] = appendUnique(result[ruleID], "unit:"+object.ID)
		}
	}
	for ruleID := range result {
		sort.Strings(result[ruleID])
	}
	return result
}

func buildRegistryItems(docs []markdownDoc, objects []ObjectView) []RegistryItem {
	items := map[string]*RegistryItem{}

	for _, doc := range docs {
		if isAppendixPath(doc.RelPath) {
			continue
		}
		kind := ""
		id := ""
		switch {
		case strings.Contains(doc.RelPath, "/units/"):
			kind = "unit"
			id = strings.TrimSpace(doc.Frontmatter.Scalars["id"])
		case strings.Contains(doc.RelPath, "/rules/"):
			kind = "rule"
			id = strings.TrimSpace(doc.Frontmatter.Scalars["rule_id"])
		}
		if kind == "" || id == "" {
			continue
		}
		key := kind + ":" + id
		item, exists := items[key]
		if !exists {
			item = &RegistryItem{ID: id, Kind: kind, Label: id}
			items[key] = item
		}
		ref := SourceRef{Path: doc.RelPath, Label: doc.Title}
		item.TruthSources = appendSourceUnique(item.TruthSources, ref)
		item.Sources = appendSourceUnique(item.Sources, ref)
		switch kind {
		case "unit":
			item.RuleRefs = appendUnique(item.RuleRefs, extractRuleIDsFromFrontmatter(doc.Frontmatter)...)
			item.UnitRefs = appendUnique(item.UnitRefs, extractUnitIDsFromFrontmatter(doc.Frontmatter)...)
		case "rule":
			item.RuleScope = inferredRuleScope(id, strings.TrimSpace(doc.Frontmatter.Scalars["rule_scope"]))
		}
	}

	result := make([]RegistryItem, 0, len(items))
	for _, item := range items {
		item.RuleRefs = sortedStrings(item.RuleRefs)
		item.UnitRefs = sortedStrings(item.UnitRefs)
		result = append(result, *item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func sourceListContainsPath(refs []SourceRef, relPath string) bool {
	for _, ref := range refs {
		if ref.Path == relPath {
			return true
		}
	}
	return false
}

func inferredRuleScope(id, declared string) string {
	declared = strings.TrimSpace(declared)
	if declared == "global" || declared == "bound" {
		return declared
	}
	if strings.HasPrefix(id, "g_rule_") {
		return "global"
	}
	if strings.HasPrefix(id, "b_rule_") {
		return "bound"
	}
	return declared
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func appendUnique(items []string, values ...string) []string {
	seen := map[string]bool{}
	for _, item := range items {
		seen[item] = true
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		items = append(items, value)
		seen[value] = true
	}
	return items
}

func appendSourceUnique(items []SourceRef, values ...SourceRef) []SourceRef {
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.Path] = true
	}
	for _, value := range values {
		if value.Path == "" || seen[value.Path] {
			continue
		}
		items = append(items, value)
		seen[value.Path] = true
	}
	return items
}

func sortedSources(sourceSet map[string]SourceRef) []SourceRef {
	result := make([]SourceRef, 0, len(sourceSet))
	for _, source := range sourceSet {
		result = append(result, source)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func sortObjects(objects []ObjectView) {
	sort.Slice(objects, func(i, j int) bool {
		if objects[i].Kind != objects[j].Kind {
			return objects[i].Kind < objects[j].Kind
		}
		return objects[i].ID < objects[j].ID
	})
}

func ptr[T any](value T) *T {
	return &value
}

func firstSource(sources []SourceRef) *SourceRef {
	if len(sources) == 0 {
		return nil
	}
	return &sources[0]
}

func isAppendixPath(path string) bool {
	return strings.Contains(path, "/appendix/")
}

func normalizeSnapshotSlices(snapshot *Snapshot) {
	if snapshot.Objects == nil {
		snapshot.Objects = []ObjectView{}
	}
	if snapshot.Registry == nil {
		snapshot.Registry = []RegistryItem{}
	}
	if snapshot.Nodes == nil {
		snapshot.Nodes = []GraphNode{}
	}
	if snapshot.Edges == nil {
		snapshot.Edges = []GraphEdge{}
	}
	if snapshot.Sources == nil {
		snapshot.Sources = []SourceRef{}
	}
	if snapshot.Diagnostics == nil {
		snapshot.Diagnostics = []Diagnostic{}
	}
	for idx := range snapshot.Objects {
		object := &snapshot.Objects[idx]
		if object.TruthPaths == nil {
			object.TruthPaths = []SourceRef{}
		}
		if object.ImplementationPaths == nil {
			object.ImplementationPaths = []SourceRef{}
		}
		if object.RuleRefs == nil {
			object.RuleRefs = []string{}
		}
		if object.UnitRefs == nil {
			object.UnitRefs = []string{}
		}
		if object.Sources == nil {
			object.Sources = []SourceRef{}
		}
	}
	for idx := range snapshot.Registry {
		item := &snapshot.Registry[idx]
		if item.TruthSources == nil {
			item.TruthSources = []SourceRef{}
		}
		if item.RuleRefs == nil {
			item.RuleRefs = []string{}
		}
		if item.UnitRefs == nil {
			item.UnitRefs = []string{}
		}
		if item.Issues == nil {
			item.Issues = []string{}
		}
		if item.Sources == nil {
			item.Sources = []SourceRef{}
		}
	}
}

type graphBuilder struct {
	nodeMap map[string]GraphNode
	edgeMap map[string]GraphEdge
}

func newGraphBuilder() *graphBuilder {
	return &graphBuilder{nodeMap: map[string]GraphNode{}, edgeMap: map[string]GraphEdge{}}
}

func (b *graphBuilder) addNode(node GraphNode) {
	if node.ID == "" {
		return
	}
	if existing, ok := b.nodeMap[node.ID]; ok {
		if existing.Source == nil && node.Source != nil {
			existing.Source = node.Source
		}
		if existing.Label == "" {
			existing.Label = node.Label
		}
		b.nodeMap[node.ID] = existing
		return
	}
	b.nodeMap[node.ID] = node
}

func (b *graphBuilder) addEdge(edge GraphEdge) {
	if edge.ID == "" || edge.From == "" || edge.To == "" {
		return
	}
	b.edgeMap[edge.ID] = edge
}

func (b *graphBuilder) nodes() []GraphNode {
	result := make([]GraphNode, 0, len(b.nodeMap))
	for _, node := range b.nodeMap {
		result = append(result, node)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (b *graphBuilder) edges() []GraphEdge {
	result := make([]GraphEdge, 0, len(b.edgeMap))
	for _, edge := range b.edgeMap {
		result = append(result, edge)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}
