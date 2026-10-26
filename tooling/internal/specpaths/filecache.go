package specpaths

import (
	"os"
	"time"
)

// Command-scoped file read cache for normalized content hashes.
//
// specflowctl runs exactly one command per process, and a command's input is
// the repository as it stood when the command started: the framework fixes the
// input snapshot before any executor reads it. Within that window the same
// evidence file must be hashed once, not once per judgment that names it — one
// verify unit's records name the same unit spec from hundreds of records, and
// re-reading it per record turned a single command into thousands of small
// file reads (see the read-count measurements on a real project: `surfaces`
// issued 8258 reads, of which about 4500 re-read 234 distinct dependency
// files).
//
// The cache is validated on every lookup: an entry whose size or modification
// time changed is re-read, so a caller that rewrites a file inside one process
// still observes the new content. ResetFileCache makes the command boundary
// explicit; the CLI calls it on entry.

type fileCacheEntry struct {
	size  int64
	mod   time.Time
	hash  string
	bytes []byte
}

var fileCache = map[string]*fileCacheEntry{}

// ResetFileCache drops every cached read. The CLI calls it once per command so
// a cached read never outlives the command that made it.
func ResetFileCache() {
	fileCache = map[string]*fileCacheEntry{}
}

// cachedFile returns the cache entry for path, re-reading the file when it is
// absent, changed on disk, or when the caller needs the content and the entry
// retained only the hash.
func cachedFile(path string, wantBytes bool) (*fileCacheEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if entry, ok := fileCache[path]; ok && entry.size == info.Size() && entry.mod.Equal(info.ModTime()) {
		if !wantBytes || entry.bytes != nil {
			return entry, nil
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	entry := &fileCacheEntry{size: info.Size(), mod: info.ModTime(), hash: HashText(data)}
	if wantBytes {
		entry.bytes = data
	}
	fileCache[path] = entry
	return entry, nil
}

// FileBytes returns a file's raw content and its normalized-content hash,
// reusing a previous read of the same unchanged file. The returned slice is
// shared with the cache: callers must treat it as read-only. Callers that also
// need the content use this instead of FileHash so one read serves both the
// hash comparison and the dependency inspection.
func FileBytes(path string) ([]byte, string, error) {
	entry, err := cachedFile(path, true)
	if err != nil {
		return nil, "", err
	}
	return entry.bytes, entry.hash, nil
}
