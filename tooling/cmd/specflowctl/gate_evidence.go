package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/contenthash"
)

// runGateEvidence computes the dependency evidence for one file read during a
// validate/verify/review run. The agent declares the line ranges it actually
// depended on (1-based, inclusive, comma-separated), and the CLI maps them
// onto the content-defined chunks they overlap. The output — the whole-file
// hash and the dependency chunk CIDs — is what gets recorded in the cache.
//
// The declared ranges are a means to an end: only the chunk CIDs are
// persisted. Line numbers are never stored, so later insertions/deletions
// cannot invalidate the dependency evidence as long as the depended-on
// content itself is unchanged.
func runGateEvidence(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-evidence", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	filePtr := fs.String("file", "", "file path relative to repo root")
	rangesPtr := fs.String("ranges", "", "line ranges actually read, e.g. 120-180,300-320 (1-based, inclusive); empty means the whole file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*filePtr) == "" {
		fmt.Fprintln(stderr, "Usage: specflowctl gate-evidence --file <path> [--ranges START-END,START-END] [--repo-root PATH]")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Computes dependency evidence for a file read during validate/verify/review.")
		fmt.Fprintln(stderr, "The agent declares the line ranges it actually depended on; the CLI maps")
		fmt.Fprintln(stderr, "them onto content-defined chunks and outputs the chunk CIDs to record")
		fmt.Fprintln(stderr, "in the cache file. An empty --ranges declares a whole-file dependency.")
		return errors.New("--file is required")
	}

	relPath := filepath.ToSlash(strings.TrimSpace(*filePtr))
	absRoot := mustAbs(*repoRootPtr)
	fullPath := filepath.Join(absRoot, filepath.FromSlash(relPath))

	text, err := contenthash.FileText(fullPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", relPath, err)
	}
	fc := contenthash.ChunkText(text)

	ranges, err := contenthash.ParseRanges(*rangesPtr)
	if err != nil {
		return err
	}
	for _, r := range ranges {
		if r[1] > fc.LineCount() {
			return fmt.Errorf("range %d-%d exceeds the file's line count (%d lines)", r[0], r[1], fc.LineCount())
		}
	}

	var deps []string
	if len(ranges) == 0 {
		// No ranges declared: the whole file is the dependency (conservative).
		for _, c := range fc.Chunks {
			deps = append(deps, c.CID)
		}
	} else {
		deps = contenthash.CIDsForRanges(fc, ranges)
	}

	if len(fc.Chunks) > 0 && len(deps) == 0 {
		return fmt.Errorf("no dependency chunks produced: the declared ranges (%s) cover no content of %s — declare the ranges that were actually read", *rangesPtr, relPath)
	}

	fmt.Fprintf(stdout, "file: %s\n", relPath)
	fmt.Fprintf(stdout, "hash: %s\n", contenthash.FileHashText(text))
	fmt.Fprintf(stdout, "lines: %d\n", fc.LineCount())
	fmt.Fprintf(stdout, "chunks: %d\n", len(fc.Chunks))
	fmt.Fprintln(stdout, "deps:")
	for _, cid := range deps {
		fmt.Fprintf(stdout, "  - %s\n", cid)
	}
	return nil
}
