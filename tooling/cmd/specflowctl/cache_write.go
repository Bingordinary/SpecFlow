package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// runCacheWrite writes a gate cache file from the agent's judgment and
// declared dependency scope, with the machine-consumed parts (whole-file hash
// and dependency CIDs) computed by the tooling. It then self-checks the
// written file with the exact chain promote and fresh use and fails (non-zero
// exit) unless the written cache is FRESH. See framework/validation_cache.md
// §Write Rules → Tooled writes for the contract.
//
// The tool replaces only the transcription of a machine-consumed format — it
// never replaces judgment. result, basis, severity counts, target, blocking,
// the findings body, and the per-check status map are all agent inputs; the
// hash and deps evidence is computed by the tooling from the declared
// sections / ranges / acceptance-items (the same declarations gate-evidence
// accepts), so a transcribed CID can never enter a cache file.
func runCacheWrite(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("cache-write", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	gatePtr := fs.String("gate", "", "gate name: validate | verify | review")
	unitPtr := fs.String("unit", "", "unit name")
	ruleIDPtr := fs.String("rule", "", "rule id")
	resultPtr := fs.String("result", "", "judgment result: pass | fail")
	basisPtr := fs.String("basis", "full", "audit metadata: full | delta | repair")
	targetPtr := fs.String("target", "", "layer checked: candidate | stable")
	blockingPtr := fs.Bool("blocking", false, "blocking declaration (required on fail caches; true iff result=fail)")
	p0Ptr := fs.Int("p0-count", 0, "P0 finding count")
	p1Ptr := fs.Int("p1-count", 0, "P1 finding count")
	p2Ptr := fs.Int("p2-count", 0, "P2 finding count")
	p3Ptr := fs.Int("p3-count", 0, "P3 finding count")
	timestampPtr := fs.String("timestamp", "", "run timestamp (default: now UTC)")
	filePtr := repeatedEntry{}
	fs.Var(&filePtr, "file", "a cache files entry declaration (repeatable): --file '<json>'")
	bodyPtr := fs.String("body", "", "cache body markdown (findings, summary)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	gate := strings.TrimSpace(*gatePtr)
	unitName := strings.TrimSpace(*unitPtr)
	ruleID := strings.TrimSpace(*ruleIDPtr)
	result := strings.TrimSpace(*resultPtr)
	target := strings.TrimSpace(*targetPtr)

	if err := requireCacheWriteTarget(gate, unitName, ruleID, target, stderr); err != nil {
		return err
	}

	targetKind := "unit"
	targetName := unitName
	if ruleID != "" {
		targetKind = "rule"
		targetName = ruleID
	}

	if result != "pass" && result != "fail" {
		return fmt.Errorf("invalid --result %q: must be pass or fail", result)
	}
	switch *basisPtr {
	case "full", "delta", "repair":
	default:
		return fmt.Errorf("invalid --basis %q: must be full, delta, or repair", *basisPtr)
	}
	if len(filePtr) == 0 {
		writeCacheWriteUsage(stderr)
		return errors.New("at least one --file entry declaration is required")
	}

	if *blockingPtr && result != "fail" {
		return errors.New("conflicting declarations: --blocking true requires --result fail (blocking means P0/P1 findings exist)")
	}
	if result == "fail" && !*blockingPtr {
		return errors.New("conflicting declarations: --result fail requires --blocking true (a failure record is blocking)")
	}

	now := strings.TrimSpace(*timestampPtr)
	if now == "" {
		now = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	}

	absRoot := mustAbs(*repoRootPtr)

	var entries []validationcache.FileEntry
	seen := make(map[string]bool)
	for _, d := range filePtr {
		path := strings.TrimSpace(d.Path)
		if path == "" {
			return errors.New("a --file entry is missing its path")
		}
		if seen[path] {
			return fmt.Errorf("duplicate --file entry for path %q", path)
		}
		seen[path] = true
		entry, err := validationcache.BuildEntry(absRoot, d)
		if err != nil {
			return err
		}
		entries = append(entries, entry)
	}

	// Status contract: a fail/blocking cache (failure record) requires a
	// per-check status map — absent status degrades the failure-recovery
	// baseline to a full re-run (fail closed, framework/validation_cache.md
	// §Format → Status declaration contract).
	if result == "fail" {
		if err := requireFailureStatuses(entries); err != nil {
			return err
		}
	}

	write := validationcache.CacheWrite{
		Command:   gate,
		Unit:      targetName,
		Mode:      "full",
		Basis:     *basisPtr,
		Result:    result,
		Target:    target,
		Blocking:  *blockingPtr,
		P0Count:   *p0Ptr,
		P1Count:   *p1Ptr,
		P2Count:   *p2Ptr,
		P3Count:   *p3Ptr,
		Timestamp: now,
		Body:      *bodyPtr,
		Entries:   entries,
	}

	writtenPath, err := validationcache.WriteCache(absRoot, targetKind, targetName, write)
	if err != nil {
		return err
	}
	// Rollback helper: a rejected self-check or appendix gate must not leave
	// a partial cache file behind — the agent only ever sees a verified write.
	rollback := func(reason string) error {
		_ = os.Remove(writtenPath)
		return fmt.Errorf("%s — the cache write at %s was rolled back", reason, relToRepo(absRoot, writtenPath))
	}

	// Self-check: the written cache must satisfy the gate's own freshness
	// chain (the exact checks promote and fresh run). A pass cache must be
	// FRESH; a failure record must classify as BLOCKED (its designed state —
	// it blocks promote and is the failure-recovery baseline). Any other
	// classification means the written cache is not accepted.
	check, err := validationcache.CheckWriteResult(absRoot, targetKind, targetName, gate, target)
	if err != nil {
		return rollback(fmt.Sprintf("self-check failed: %v", err))
	}
	expectedBlocked := result == "fail"
	accepted := check.Fresh
	if expectedBlocked && check.Category == validationcache.CategoryBlocked {
		accepted = true
	}
	if !accepted {
		return rollback(fmt.Sprintf("cache-write self-check FAILED: %s — the written cache is not accepted by the %s gate", check.Reason, gate))
	}
	if expectedBlocked {
		fmt.Fprintf(stdout, "Self-check: BLOCKED (result: fail — the failure record blocks promote and is the failure-recovery baseline)\n")
	} else {
		fmt.Fprintf(stdout, "Self-check: FRESH (result: %s, dependency chunks of %d file(s) unchanged)\n", result, len(entries))
	}

	// Appendix coverage: a pass validate cache must list every non-exempt
	// candidate appendix (the promote appendix gate). A failure record is
	// exempt — the appendix gate reads only pass-result validate caches, and
	// a failure record blocks promote regardless.
	if gate == "validate" && targetKind == "unit" && target == "candidate" && result == "pass" {
		appendixResult, aerr := validationcache.CheckAppendicesInCache(absRoot, unitName)
		if aerr != nil {
			return rollback(fmt.Sprintf("appendix coverage check failed: %v", aerr))
		}
		if !appendixResult.Fresh {
			return rollback(fmt.Sprintf("cache-write rejected: %s", appendixResult.Reason))
		}
	}

	fmt.Fprintf(stdout, "Cache written: %s\n", relToRepo(absRoot, writtenPath))
	return nil
}

// requireCacheWriteTarget validates the gate/target combination and prints
// usage on invalid input. Rule targets have validate only (rule verify and
// review have been removed).
func requireCacheWriteTarget(gate, unitName, ruleID, target string, stderr io.Writer) error {
	switch gate {
	case "validate", "verify", "review":
	default:
		writeCacheWriteUsage(stderr)
		return fmt.Errorf("invalid --gate %q: must be validate, verify, or review", gate)
	}
	if unitName != "" && ruleID != "" {
		writeCacheWriteUsage(stderr)
		return errors.New("--unit and --rule are mutually exclusive")
	}
	if unitName == "" && ruleID == "" {
		writeCacheWriteUsage(stderr)
		return errors.New("--unit or --rule is required")
	}
	if target != "candidate" && target != "stable" {
		writeCacheWriteUsage(stderr)
		return fmt.Errorf("invalid --target %q: must be candidate or stable", target)
	}
	if ruleID != "" && gate != "validate" {
		return fmt.Errorf("rule targets support the validate gate only (rule verify and review have been removed) — got %q", gate)
	}
	return nil
}

// requireFailureStatuses verifies that a failure-record cache carries a
// per-check status map with valid values. The status map is the
// failure-recovery baseline; a record without it fails closed (the recovery
// degrades to a full re-run).
func requireFailureStatuses(entries []validationcache.FileEntry) error {
	statusSeen := false
	for _, e := range entries {
		for _, c := range e.Checks {
			statusSeen = true
			switch c.Status {
			case "pass", "fail", "carried":
			default:
				return fmt.Errorf("failure record entry %q check %q has invalid status %q: must be pass, fail, or carried", e.Path, c.Check, c.Status)
			}
		}
	}
	if !statusSeen {
		return errors.New("a failure record (result: fail) requires a per-check status map — declare the status (pass|fail|carried) on every check in the --file entries")
	}
	return nil
}

// repeatedEntry is a repeatable --file flag value carrying raw JSON entry
// declarations.
type repeatedEntry []validationcache.EntryDeclaration

func (r *repeatedEntry) String() string {
	return fmt.Sprintf("%d file declaration(s)", len(*r))
}

func (r *repeatedEntry) Set(v string) error {
	var d validationcache.EntryDeclaration
	if err := json.Unmarshal([]byte(v), &d); err != nil {
		return fmt.Errorf("invalid --file JSON %q: %w", v, err)
	}
	*r = append(*r, d)
	return nil
}

func relToRepo(absRoot, path string) string {
	rel, err := filepath.Rel(absRoot, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

func writeCacheWriteUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  specflowctl cache-write --gate validate|verify|review (--unit NAME | --rule ID) --result pass|fail [flags]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Writes a gate cache file with the machine-consumed evidence (whole-file hash")
	fmt.Fprintln(w, "and dependency CIDs) computed by the tooling, then self-checks the written file")
	fmt.Fprintln(w, "with the exact chain promote and fresh use. The tool replaces only the")
	fmt.Fprintln(w, "transcription of a machine-consumed format — the agent's judgment (result,")
	fmt.Fprintln(w, "basis, severity counts, findings body, per-check status) is never replaced.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "--file JSON accepts the entry's declaration (repeatable):")
	fmt.Fprintln(w, `  {"path":"src/auth/login.go","ranges":"120-180","sections":["Description"],"checks":[{"check":"5","status":"pass","sections":["Description"]}]}`)
	fmt.Fprintln(w, "  path: physical repo path or logical reference (unit:NAME, unit:NAME:appendix:BASE, rule:ID)")
fmt.Fprintln(w, "  sections / ranges / acceptance_items: the declared dependency scope (same grammar")
fmt.Fprintln(w, "  as gate-evidence); empty means the whole file (conservative). sections accepts the")
fmt.Fprintln(w, "  reserved spelling \"frontmatter\" naming the pre-## region (same as gate-evidence --section).")
fmt.Fprintln(w, "  checks is optional; when present, every check's deps join the file-level deps")
fmt.Fprintln(w, "  union (union discipline). A failure record (--result fail) requires a per-check")
fmt.Fprintln(w, "  status (pass|fail|carried).")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --gate GATE      validate | verify | review (required)")
	fmt.Fprintln(w, "  --unit NAME      Unit name (mutually exclusive with --rule)")
	fmt.Fprintln(w, "  --rule ID        Rule id (validate only)")
	fmt.Fprintln(w, "  --result R       pass | fail (required)")
	fmt.Fprintln(w, "  --basis B        full | delta | repair (default: full)")
	fmt.Fprintln(w, "  --target T       candidate | stable (required)")
	fmt.Fprintln(w, "  --blocking       blocking declaration (required on fail caches; true iff result=fail)")
	fmt.Fprintln(w, "  --p0-count N      severity counts (default 0)")
	fmt.Fprintln(w, "  --p1-count N")
	fmt.Fprintln(w, "  --p2-count N")
	fmt.Fprintln(w, "  --p3-count N")
	fmt.Fprintln(w, "  --timestamp T    run timestamp RFC3339 (default: now UTC)")
	fmt.Fprintln(w, "  --file JSON      one cache files entry declaration (repeatable)")
	fmt.Fprintln(w, "  --body TEXT      cache body markdown")
	fmt.Fprintln(w, "  --repo-root PATH Repository root path (default: .)")
}
