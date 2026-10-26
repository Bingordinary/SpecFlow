package specpaths

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// NormalizeText normalizes line endings and ensures a trailing newline.
// CRLF→LF, standalone CR→LF, append \n if missing.
// This is the canonical normalization used by specFlow for content-addressed
// freshness checks across review fingerprints and validation caches.
func NormalizeText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text
}

// FileHash computes the SHA-256 hash of a file's normalized content.
// Uses NormalizeText to ensure cross-platform consistency regardless of
// git autocrlf settings or the host operating system.
// Repeated hashes of the same unchanged file inside one command reuse the
// first read (see filecache.go).
func FileHash(path string) (string, error) {
	entry, err := cachedFile(path, false)
	if err != nil {
		return "", err
	}
	return entry.hash, nil
}

// HashText computes the same hash FileHash produces for the same bytes, so a
// caller that has already read a file's content hashes it without a second
// read of the same path.
func HashText(data []byte) string {
	normalized := NormalizeText(string(data))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
