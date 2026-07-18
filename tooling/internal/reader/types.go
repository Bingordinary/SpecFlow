package reader

type Snapshot struct {
	Version     int64        `json:"version"`
	GeneratedAt string       `json:"generated_at"`
	Project     ProjectInfo  `json:"project"`
	Objects     []ObjectView `json:"objects"`
	Registry    []RegistryItem `json:"registry"`
	Nodes       []GraphNode  `json:"nodes"`
	Edges       []GraphEdge  `json:"edges"`
	Sources     []SourceRef  `json:"sources"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type ProjectInfo struct {
	RepoRoot         string `json:"repo_root"`
	RuleBaselineFile string `json:"rule_baseline_file"`
	UnitCount        int    `json:"unit_count"`
	RuleCount        int    `json:"rule_count"`
	TruthFileCount   int    `json:"truth_file_count"`
}

type ObjectView struct {
	ID                  string      `json:"id"`
	Kind                string      `json:"kind"`
	Label               string      `json:"label"`
	Responsibility      string      `json:"responsibility,omitempty"`
	Version             string      `json:"version,omitempty"`
	Layer               string      `json:"layer,omitempty"`
	HasCandidate        bool        `json:"has_candidate"`
	HasStable           bool        `json:"has_stable"`
	TruthPaths          []SourceRef `json:"truth_paths"`
	ImplementationPaths []SourceRef `json:"implementation_paths"`
	RuleRefs            []string    `json:"rule_refs"`
	UnitRefs            []string    `json:"unit_refs"`
	Sources             []SourceRef `json:"sources"`
}

type RegistryItem struct {
	ID             string      `json:"id"`
	Kind           string      `json:"kind"`
	Label          string      `json:"label"`
	RuleScope      string      `json:"rule_scope,omitempty"`
	TruthSources   []SourceRef `json:"truth_sources"`
	RuleRefs       []string    `json:"rule_refs"`
	UnitRefs       []string    `json:"unit_refs"`
	Issues         []string    `json:"issues"`
	Sources        []SourceRef `json:"sources"`
}

type GraphNode struct {
	ID     string     `json:"id"`
	Kind   string     `json:"kind"`
	Label  string     `json:"label"`
	Group  string     `json:"group"`
	Source *SourceRef `json:"source,omitempty"`
}

type GraphEdge struct {
	ID     string     `json:"id"`
	From   string     `json:"from"`
	To     string     `json:"to"`
	Kind   string     `json:"kind"`
	Label  string     `json:"label"`
	Source *SourceRef `json:"source,omitempty"`
}

type SourceRef struct {
	Path  string `json:"path"`
	Line  int    `json:"line,omitempty"`
	Label string `json:"label,omitempty"`
}

type Diagnostic struct {
	Severity string     `json:"severity"`
	Message  string     `json:"message"`
	Source   *SourceRef `json:"source,omitempty"`
}

type SourceDiff struct {
	Available     bool        `json:"available"`
	CandidatePath string      `json:"candidate_path,omitempty"`
	StablePath    string      `json:"stable_path,omitempty"`
	Reason        string      `json:"reason,omitempty"`
	Summary       DiffSummary `json:"summary"`
	Hunks         []DiffHunk  `json:"hunks"`
}

type DiffSummary struct {
	Added    int `json:"added"`
	Deleted  int `json:"deleted"`
	Modified int `json:"modified"`
	Hunks    int `json:"hunks"`
}

type DiffHunk struct {
	StableStart    int        `json:"stable_start"`
	CandidateStart int        `json:"candidate_start"`
	Lines          []DiffLine `json:"lines"`
}

type DiffLine struct {
	Type          string `json:"type"`
	StableLine    int    `json:"stable_line,omitempty"`
	CandidateLine int    `json:"candidate_line,omitempty"`
	Text          string `json:"text"`
}
