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
//
// With --acceptance-items, the acceptance_item_set structural region (see
// contenthash.AcceptanceItemsRegion) is declared as a structural dependency:
// the region's content CID is emitted with a `region:acceptance_items:` tag.
// Structural regions are located by structure rather than line numbers, so
// edits outside the region — even inside the same content-defined chunk —
// do not invalidate the dependency. This is the precise declaration mode for
// cross-unit checks, which depend only on a dependency unit's acceptance
// item set.
//
// With --section <heading>, a section region (see contenthash.SectionRegions)
// is declared as a structural dependency: the region's content CID is emitted
// with a `region:section:<heading>:<cid>` tag. Section regions are located by
// heading text rather than line numbers, so edits in other sections — even
// inside the same content-defined chunk — do not invalidate the dependency.
// --section is repeatable; the declared regions are emitted together with the
// chunk CIDs as one deps list.
//
// With --sections, every section region of the file is listed (heading, line
// range, content CID) without declaring anything — the informational output
// that lets an agent name the exact section regions its judgment depends on.
func runGateEvidence(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("gate-evidence", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repoRootPtr := fs.String("repo-root", ".", "repository root")
	filePtr := fs.String("file", "", "file path relative to repo root")
	rangesPtr := fs.String("ranges", "", "line ranges actually read, e.g. 120-180,300-320 (1-based, inclusive); empty means the whole file")
	acceptanceItemsPtr := fs.Bool("acceptance-items", false, "declare the acceptance_item_set structural region as the dependency (with no --ranges it replaces the whole-file declaration)")
	sectionsPtr := fs.Bool("sections", false, "list every section region of the file (heading, line range, CID) without declaring dependencies")
	sectionPtr := repeatedString{}
	fs.Var(&sectionPtr, "section", "declare a section region as the dependency by heading text (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(*filePtr) == "" {
		fmt.Fprintln(stderr, "Usage: specflowctl gate-evidence --file <path> [--ranges START-END,START-END] [--acceptance-items] [--section HEADING]... [--sections] [--repo-root PATH]")
		fmt.Fprintln(stderr, "")
		fmt.Fprintln(stderr, "Computes dependency evidence for a file read during validate/verify/review.")
		fmt.Fprintln(stderr, "The agent declares the line ranges it actually depended on; the CLI maps")
		fmt.Fprintln(stderr, "them onto content-defined chunks and outputs the chunk CIDs to record")
		fmt.Fprintln(stderr, "in the cache file. An empty --ranges declares a whole-file dependency.")
		fmt.Fprintln(stderr, "--acceptance-items declares the acceptance_item_set structural region")
		fmt.Fprintln(stderr, "(structure-located, independent of chunk boundaries and line numbers) as the")
		fmt.Fprintln(stderr, "dependency. With an empty --ranges the region replaces the whole-file")
		fmt.Fprintln(stderr, "declaration; with --ranges both are declared.")
		fmt.Fprintln(stderr, "--section HEADING declares the section region with that heading text")
		fmt.Fprintln(stderr, "(structure-located by heading, independent of line numbers) as the")
		fmt.Fprintln(stderr, "dependency; repeatable. --sections lists every section region without")
		fmt.Fprintln(stderr, "declaring dependencies — use its output to name --section values.")
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
	if len(ranges) == 0 && !*acceptanceItemsPtr && len(sectionPtr) == 0 && !*sectionsPtr {
		// Nothing declared: the whole file is the dependency (conservative).
		for _, c := range fc.Chunks {
			deps = append(deps, c.CID)
		}
	} else {
		if len(ranges) > 0 {
			deps = contenthash.CIDsForRanges(fc, ranges)
		}
		if *acceptanceItemsPtr {
			region, ok := contenthash.AcceptanceItemsRegion(text)
			if !ok {
				return fmt.Errorf("acceptance_item_set region not found in %s — cannot declare the structural dependency", relPath)
			}
			deps = append(deps, "region:acceptance_items:"+contenthash.RegionCID(region))
		}
		for _, heading := range sectionPtr {
			// The --sections output names the frontmatter region "frontmatter";
			// accept that spelling as the empty-heading declaration.
			if heading == "frontmatter" {
				// "frontmatter" is a reserved spelling for the pre-heading
				// region — a real section with that heading text would make
				// the declaration silently bind to the wrong region.
				if _, ok := contenthash.LocateSectionRegion(text, "frontmatter"); ok {
					return fmt.Errorf("reserved heading %q: the file has a real ## frontmatter section, which cannot be declared by --section frontmatter (the spelling names the pre-heading region) — rename the section", heading)
				}
				heading = ""
			}
			region, ok := contenthash.LocateSectionRegion(text, heading)
			if !ok {
				return fmt.Errorf("section %q not found in %s (or declared more than once) — list the sections with --sections", heading, relPath)
			}
			deps = append(deps, "region:section:"+heading+":"+contenthash.RegionCID(region.Text))
		}
	}

	if len(fc.Chunks) > 0 && len(deps) == 0 && !*sectionsPtr {
		return fmt.Errorf("no dependency chunks produced: the declared ranges (%s) cover no content of %s — declare the ranges that were actually read", *rangesPtr, relPath)
	}

	fmt.Fprintf(stdout, "file: %s\n", relPath)
	fmt.Fprintf(stdout, "hash: %s\n", contenthash.FileHashText(text))
	fmt.Fprintf(stdout, "lines: %d\n", fc.LineCount())
	fmt.Fprintf(stdout, "chunks: %d\n", len(fc.Chunks))
	if *sectionsPtr {
		fmt.Fprintln(stdout, "sections:")
		for _, r := range contenthash.SectionRegions(text) {
			heading := r.Heading
			if heading == "" {
				heading = "frontmatter"
			}
			fmt.Fprintf(stdout, "  - heading: %s\n", heading)
			fmt.Fprintf(stdout, "    lines: %d-%d\n", r.Start, r.End)
			fmt.Fprintf(stdout, "    cid: %s\n", contenthash.RegionCID(r.Text))
		}
	}
	fmt.Fprintln(stdout, "deps:")
	for _, cid := range deps {
		fmt.Fprintf(stdout, "  - %s\n", cid)
	}
	return nil
}

// repeatedString is a repeatable string flag value.
type repeatedString []string

func (r *repeatedString) String() string {
	return strings.Join(*r, ",")
}

func (r *repeatedString) Set(v string) error {
	*r = append(*r, v)
	return nil
}
