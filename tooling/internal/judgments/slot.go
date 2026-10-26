package judgments

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Slot returns the store-relative directory that holds one judgment key's
// records in one spec context:
//
//	<protocol>/<kind>/<unit>/<subject>/<context>
//
// The slot is a function of the record's own identity fields, so a query that
// already knows the key — "the public code judgment for this file", "the item
// decision for this acceptance item in the spec content I am binding" — computes
// the directory instead of reading records to discover which ones apply. The
// directory holds the record files named by content hash, and, for a key with a
// current decision, a `current` pointer naming the record in force.
//
// Encoding rules, one per component:
//
//   - protocol is the judging-standard fingerprint; a record whose standard
//     changed is never valid again, so the fingerprint is the top level and a
//     whole stale standard is dropped by removing one directory.
//   - kind is the record kind, so a caller that wants one kind never walks the
//     others.
//   - unit is "_" when the key is not unit-bound (a public code judgment).
//   - subject is the reviewed repository path for code and design, "_" for a
//     kind whose subject is its unit, and a digest for an item, whose subject is
//     free text that need not be a legal path component on every platform.
//   - context is the spec-context fingerprint an item decision was made in, and
//     "_" for every kind whose decision is not bound to one spec content.
func Slot(protocol, kind, unit, subject, context string) (string, error) {
	// The protocol is a fingerprint in a real store; it is checked as a path
	// component, not as a digest, so a store written by an older or fixture
	// build is still addressable and stays collectable.
	if !validPathComponent(protocol) {
		return "", fmt.Errorf("invalid judgment protocol %q", protocol)
	}
	if !knownKind(kind) {
		return "", fmt.Errorf("unknown judgment kind %q", kind)
	}
	unitPart, err := nameComponent(unit)
	if err != nil {
		return "", fmt.Errorf("judgment unit: %w", err)
	}
	subjectPart, err := subjectComponent(kind, subject)
	if err != nil {
		return "", err
	}
	contextPart, err := contextComponent(context)
	if err != nil {
		return "", err
	}
	return protocol + "/" + kind + "/" + unitPart + "/" + subjectPart + "/" + contextPart, nil
}

// SlotProtocol returns the judging-standard fingerprint a slot is filed under.
// A record's standard is part of where it lives, so a collection pass reads it
// from the path instead of from the record body.
func SlotProtocol(slot string) (string, error) {
	if err := validateSlot(slot); err != nil {
		return "", err
	}
	protocol, _, _ := strings.Cut(slot, "/")
	return protocol, nil
}

// SlotOf returns the slot for a record, from the record's own identity fields.
func SlotOf(r *Record) (string, error) {
	return Slot(r.Protocol, r.Kind, r.Unit, r.Subject, r.SpecContext)
}

// nameComponent encodes a unit name: the empty name is "_", and a name is used
// as written only when it is a single path component.
func nameComponent(name string) (string, error) {
	if name == "" {
		return "_", nil
	}
	if !validPathComponent(name) {
		return "", fmt.Errorf("invalid name %q", name)
	}
	return name, nil
}

// subjectComponent encodes a subject: a repository path for code and design, a
// digest for free-text item names, and "_" for a kind whose subject is its unit.
func subjectComponent(kind, subject string) (string, error) {
	switch kind {
	case Code, Design:
		if strings.TrimSpace(subject) == "" {
			return "", fmt.Errorf("a %s judgment requires a subject path", kind)
		}
		parts := strings.Split(subject, "/")
		for _, part := range parts {
			if !validPathComponent(part) {
				return "", fmt.Errorf("subject path %q is not a repository-relative path", subject)
			}
		}
		return strings.Join(parts, "/"), nil
	case Item:
		if strings.TrimSpace(subject) == "" {
			return "", fmt.Errorf("an item judgment requires a subject")
		}
		return ItemSubjectKey(subject), nil
	default:
		// architecture and unit: the subject is the unit, already in the path.
		return "_", nil
	}
}

// ItemSubjectKey encodes an acceptance item name as one path component. An
// item name is free text — it may contain spaces, separators or non-ASCII
// characters — so the directory uses a digest of the name and the name itself
// stays in the record, where it is read rather than parsed as a path.
func ItemSubjectKey(subject string) string {
	sum := sha256.Sum256([]byte(subject))
	return "h" + hex.EncodeToString(sum[:16])
}

// contextComponent encodes the spec-context fingerprint: "_" when the record is
// not bound to one spec content, the fingerprint otherwise.
func contextComponent(context string) (string, error) {
	if context == "" {
		return "_", nil
	}
	if !idPattern.MatchString(context) {
		return "", fmt.Errorf("invalid spec context fingerprint %q", context)
	}
	return context, nil
}

// validPathComponent reports whether name can be used as one path element
// without changing the directory it names.
func validPathComponent(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\\`) {
		return false
	}
	return true
}
