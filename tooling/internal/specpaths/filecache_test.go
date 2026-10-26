package specpaths

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFileAt(t *testing.T, path string, content string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
}

// TestFileHashReusesAnUnchangedRead pins the one-read-per-command property: a
// file rewritten with the same size and the same modification time is served
// from the cached read, and ResetFileCache is what makes the next lookup read
// the file again.
func TestFileHashReusesAnUnchangedRead(t *testing.T) {
	ResetFileCache()
	path := filepath.Join(t.TempDir(), "evidence.md")
	mtime := time.Unix(1_700_000_000, 0)

	writeFileAt(t, path, "aaaa\n", mtime)
	first, err := FileHash(path)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	if want := HashText([]byte("aaaa\n")); first != want {
		t.Fatalf("first hash = %q, want %q", first, want)
	}

	// Same size, same mtime: the cached read must answer.
	writeFileAt(t, path, "bbbb\n", mtime)
	cached, err := FileHash(path)
	if err != nil {
		t.Fatalf("cached hash: %v", err)
	}
	if cached != first {
		t.Fatalf("cached hash = %q, want the first hash %q", cached, first)
	}

	ResetFileCache()
	fresh, err := FileHash(path)
	if err != nil {
		t.Fatalf("hash after reset: %v", err)
	}
	if want := HashText([]byte("bbbb\n")); fresh != want {
		t.Fatalf("hash after reset = %q, want %q", fresh, want)
	}
}

// TestFileHashObservesAChangedFile pins the validation half: a rewrite that
// changes size or modification time is re-read, so a command never serves a
// stale hash for a file another step in the same process just wrote.
func TestFileHashObservesAChangedFile(t *testing.T) {
	ResetFileCache()
	path := filepath.Join(t.TempDir(), "evidence.md")
	base := time.Unix(1_700_000_000, 0)

	writeFileAt(t, path, "aaaa\n", base)
	first, err := FileHash(path)
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}

	writeFileAt(t, path, "aaaaaaaa\n", base)
	changedSize, err := FileHash(path)
	if err != nil {
		t.Fatalf("hash after size change: %v", err)
	}
	if want := HashText([]byte("aaaaaaaa\n")); changedSize != want {
		t.Fatalf("hash after size change = %q, want %q", changedSize, want)
	}
	if changedSize == first {
		t.Fatal("size change was not observed")
	}

	writeFileAt(t, path, "bbbbbbbb\n", base.Add(time.Second))
	changedTime, err := FileHash(path)
	if err != nil {
		t.Fatalf("hash after mtime change: %v", err)
	}
	if want := HashText([]byte("bbbbbbbb\n")); changedTime != want {
		t.Fatalf("hash after mtime change = %q, want %q", changedTime, want)
	}
}

func TestFileBytesReturnsContentAndHash(t *testing.T) {
	ResetFileCache()
	path := filepath.Join(t.TempDir(), "evidence.md")
	writeFileAt(t, path, "alpha\r\n", time.Time{})

	data, hash, err := FileBytes(path)
	if err != nil {
		t.Fatalf("file bytes: %v", err)
	}
	if string(data) != "alpha\r\n" {
		t.Fatalf("content = %q, want the raw file bytes", string(data))
	}
	if want := HashText([]byte("alpha\r\n")); hash != want {
		t.Fatalf("hash = %q, want %q", hash, want)
	}

	// The hash FileBytes reports is the hash FileHash reports for the same file.
	again, err := FileHash(path)
	if err != nil {
		t.Fatalf("file hash: %v", err)
	}
	if again != hash {
		t.Fatalf("FileHash = %q, FileBytes hash = %q", again, hash)
	}
}

func TestFileReadsReportMissingFiles(t *testing.T) {
	ResetFileCache()
	path := filepath.Join(t.TempDir(), "absent.md")

	if _, err := FileHash(path); err == nil {
		t.Fatal("FileHash on a missing file must fail")
	}
	if _, _, err := FileBytes(path); err == nil {
		t.Fatal("FileBytes on a missing file must fail")
	}
}
