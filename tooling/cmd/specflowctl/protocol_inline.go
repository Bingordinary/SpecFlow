package main

import (
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

// The session-to-section mapping lives in `judgments` so the inlined mission
// text and the check-section part of a standard fingerprint come from one
// source: an edit that changes what a reviewer reads must change the value the
// cache binds, and two implementations could drift apart silently.

// protocolSectionRef is one framework document section a mission inlines.
type protocolSectionRef = judgments.ProtocolSection

// protocolChunk is one inlined framework section inside a rendered mission.
type protocolChunk = judgments.ProtocolChunk

// protocolSectionsFor maps a session to the framework sections its mission
// must carry inline.
func protocolSectionsFor(gate, targetKind, kind string, keys []string) ([]protocolSectionRef, error) {
	return judgments.ProtocolSectionsFor(gate, targetKind, kind, keys)
}

// finalSynthesisSections lists the framework sections the final synthesis
// judges by.
func finalSynthesisSections() []protocolSectionRef {
	return judgments.FinalSynthesisSections()
}

// inlineProtocol renders the requested framework sections as mission chunks.
func inlineProtocol(root, frameworkRoot string, sections []protocolSectionRef) ([]protocolChunk, error) {
	return judgments.InlineProtocol(root, frameworkRoot, sections)
}

// stripAtomMarkers removes atom delivery marker lines from an inlined section.
func stripAtomMarkers(body string) string {
	return judgments.StripAtomMarkers(body)
}
