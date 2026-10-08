package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
)

func synthesisPromote(t *testing.T, root, unit string) {
	t.Helper()
	sharedValidateFixture(t, root, unit)
	var out, errOut bytes.Buffer
	if err := runPromote([]string{"--repo-root", root, "--unit", unit}, &out, &errOut); err != nil {
		t.Fatalf("promote %s: %v %s", unit, err, out.String())
	}
}

func synthesisItemRecord(t *testing.T, root, unit string) *judgments.Record {
	t.Helper()
	state := grReadJudgmentBaseline(t, root, "unit", unit, "verify")
	r, err := judgments.Load(root, state.Records["item:"+unit+":"+unit+".core"].Reference)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func TestItemRequiredCodeEvidenceCannotBeOmittedFromDependencies(t *testing.T) {
	root, main, _ := sharedFixture(t)
	grWriteFile(t, root, "agreement.js", "export const tokenRequired = true;\n")
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate", "--inputs-file", grInputsManifest(t, "agreement.js"))
	key := "item:auth:auth.core"
	report := grVerifyItemBody(key, "ALIGNED", "contracts.js:1") + key + ": " + main + ": acceptance_item:auth.core\n"
	if err := sharedSubmit(t, root, id, key, report); err != nil {
		t.Fatal(err)
	}
	sharedFinish(t, root, id)
	record := synthesisItemRecord(t, root, "auth")
	for _, path := range []string{"contracts.js", "agreement.js"} {
		found := false
		for _, dep := range record.Dependencies {
			if dep.Path == path && dep.Hash != "" {
				found = true
			}
		}
		if !found {
			t.Fatalf("required code input %s has no whole-file dependency", path)
		}
	}
}
func TestDivergenceAnalysisCompletesBeforeItemAcceptance(t *testing.T) {
	root, main, _ := sharedFixture(t)
	grEnableMissionLayout(t, root)
	id := grPlan(t, root, "--gate", "verify", "--unit", "auth", "--target", "candidate")
	var out, errOut bytes.Buffer
	args := []string{"--repo-root", root, "--run", id, "--keys", "item:auth:auth.core", "--format", "prompt"}
	if err := runGateMission(args, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Steps 1-7") {
		t.Fatal("initial item mission omits divergence analysis")
	}
	grSubmitOK(t, root, id, "auth.core", grVerifyMismatchReport("auth.core", main, "contracts.js", "P1"))
	if err := runGateMission(args, &out, &errOut); err == nil {
		t.Fatal("accepted item task was reopened for a second analysis")
	}
	doc, err := os.ReadFile("../../../framework/unit_verify_checklist.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(doc), "The tool supplies the accepted alignment result and report") {
		t.Fatal("Step 7 still requests a mission after item acceptance")
	}
}
