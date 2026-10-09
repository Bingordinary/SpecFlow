package validationcache

import (
	"strings"
	"testing"
)

func TestLayerRewritePreservesPrefixPeerEvidence(t *testing.T) {
	for _, source := range []string{"candidate", "stable"} {
		t.Run(source, func(t *testing.T) {
			own := "docs/specs/units/" + source + "/appendix/unit_auth_protocol.md"
			peer := "docs/specs/units/" + source + "/appendix/unit_auth_extra_protocol.md"
			input := "---\ncommand: verify\nunit: auth\nresult: pass\ntarget: " + source + "\nfiles:\n  - path: " + own + "\n    hash: sha256:own\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n  - path: " + peer + "\n    hash: sha256:peer\n    chunker: buzhash-v1\n    chunks:\n      - cid: sha256:fixture\n        start: 1\n        end: 1\n---\n"
			destination := "stable"
			rewrite := rewriteCacheLayerToStable
			if source == "stable" {
				destination, rewrite = "candidate", rewriteCacheLayer
			}
			out, changed := rewrite(input, []string{own})
			if !changed || !strings.Contains(out, strings.Replace(own, "/"+source+"/", "/"+destination+"/", 1)) {
				t.Fatal("owned appendix did not follow its unit's layer")
			}
			if !strings.Contains(out, peer) || strings.Contains(out, strings.Replace(peer, "/"+source+"/", "/"+destination+"/", 1)) {
				t.Fatal("peer evidence was rebound to another layer")
			}
		})
	}
}
