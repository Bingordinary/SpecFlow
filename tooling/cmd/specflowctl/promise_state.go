package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// writePromiseStateDetail renders the complete promise state of one unit layer
// inside the `fresh` detail view: contract closure, own promise fingerprints,
// coverage, graph, dependency pins, and the gate evidence the done state rests
// on. It is the only diagnostic view; only a passing verify finalize records
// the promise state, so a recorded pin always names a contract a verification
// actually ran against.
func writePromiseStateDetail(stdout io.Writer, report *promiseStateReport) {
	if len(report.ClosureReasons) > 0 {
		fmt.Fprintf(stdout, "promise %s — %s\n", report.Closure, strings.Join(report.ClosureReasons, "; "))
	} else {
		fmt.Fprintf(stdout, "promise %s\n", report.Closure)
	}

	if len(report.Own) == 0 {
		fmt.Fprintln(stdout, "  own promises: (none)")
	} else {
		fmt.Fprintln(stdout, "  own promises:")
		for _, id := range sortedKeys(report.Own) {
			fmt.Fprintf(stdout, "    %s  %s\n", id, report.Own[id])
		}
	}
	fmt.Fprintf(stdout, "  coverage: %s — %s\n", report.Coverage.Status, report.Coverage.Details)
	fmt.Fprintf(stdout, "  graph: %s — %s\n", report.Graph.Status, report.Graph.Details)
	if len(report.CurrentIssues) > 0 {
		fmt.Fprintf(stdout, "  current issues: %s\n", strings.Join(report.CurrentIssues, "; "))
	}

	switch {
	case !report.StateRecorded:
		fmt.Fprintln(stdout, "  pins: not recorded")
	case len(report.State.Pins) == 0:
		fmt.Fprintln(stdout, "  pins: (none)")
	default:
		fmt.Fprintln(stdout, "  pins:")
		for _, entry := range report.PinStates {
			fmt.Fprintf(stdout, "    %s  %s%s\n", entry.ID, entry.Status, promiseStateReason(entry.Reason))
		}
	}
	if report.Layer == "candidate" {
		switch {
		case len(report.PublicationIssues) > 0:
			fmt.Fprintf(stdout, "  publication: INVALID — %s\n", strings.Join(report.PublicationIssues, "; "))
		case len(report.State.Pins) > 0:
			fmt.Fprintln(stdout, "  publication:")
			for _, entry := range report.Publication {
				fmt.Fprintf(stdout, "    %s  %s%s\n", entry.ID, entry.Status, promiseStateReason(entry.Reason))
			}
		}
	}
	fmt.Fprintf(stdout, "  evidence: validate %s; verify %s\n", report.ValidateCheck.Category, report.VerifyCheck.Category)

	if report.Done {
		fmt.Fprintln(stdout, "done    YES")
		return
	}
	fmt.Fprintf(stdout, "done    NO — %s\n", strings.Join(report.DoneReasons, "; "))
}

// evidenceChecks reads the layer's validate and verify cache states. These are
// the existing code-evidence gates; the promise view adds the contract-level
// closure on top of them.
func evidenceChecks(repoRoot, unitName, layer string, derivation *gaterun.Derivation) (validationcache.CheckResult, validationcache.CheckResult, error) {
	if layer == "stable" {
		validateCheck, err := validationcache.CheckValidateStable(repoRoot, unitName)
		if err != nil {
			return validationcache.CheckResult{}, validationcache.CheckResult{}, err
		}
		verifyCheck, err := checkStableUnitVerifyMerged(derivation, repoRoot, unitName)
		if err != nil {
			return validationcache.CheckResult{}, validationcache.CheckResult{}, err
		}
		return validateCheck, verifyCheck, nil
	}
	validateCheck, err := validationcache.CheckValidate(repoRoot, unitName)
	if err != nil {
		return validationcache.CheckResult{}, validationcache.CheckResult{}, err
	}
	verifyCheck, err := checkUnitVerifyMerged(derivation, repoRoot, unitName, "candidate")
	if err != nil {
		return validationcache.CheckResult{}, validationcache.CheckResult{}, err
	}
	return validateCheck, verifyCheck, nil
}

// promiseClosure reports contract closure. It intentionally does not report
// code verification; the caller composes it with the gate caches. A layer with
// no recorded state is OPEN even when it declares no promises: the state
// record is what makes the contract version auditable.
func promiseClosure(state *validationcache.PromiseStateFile, stateRecorded bool, unitName, layer string, promises []specvalidation.Promise, own, current map[string]string, coverage, graph specvalidation.CheckResult) (string, []string) {
	closure := "CLOSED"
	var reasons []string
	if !stateRecorded {
		closure = "OPEN"
		reasons = append(reasons, "promise state not recorded")
	}
	if identityIssues := validationcache.CheckPromiseStateIdentity(state, unitName, layer); len(identityIssues) > 0 {
		closure = "OPEN"
		reasons = append(reasons, identityIssues...)
	}
	if coverage.Status != specvalidation.Pass {
		closure = "OPEN"
		reasons = append(reasons, "promise coverage failed")
	}
	if graph.Status != specvalidation.Pass {
		closure = "OPEN"
		reasons = append(reasons, "promise graph failed")
	}
	if len(state.Promises) == 0 && len(own) > 0 {
		closure = "OPEN"
		reasons = append(reasons, "state records no own promises")
	}
	recordedOwn := map[string]bool{}
	for _, pin := range state.Promises {
		recordedOwn[pin.ID] = true
		if fingerprint, ok := own[pin.ID]; !ok || fingerprint != pin.Fingerprint {
			closure = "OPEN"
			reasons = append(reasons, "own promise changed: "+pin.ID)
		}
	}
	for id := range own {
		if !recordedOwn[id] {
			closure = "OPEN"
			reasons = append(reasons, "own promise not recorded: "+id)
		}
	}
	requiredPins := map[string]bool{}
	for _, promise := range promises {
		for _, dep := range promise.DependsOn {
			requiredPins[dep] = true
		}
	}
	recordedPins := map[string]bool{}
	for _, pin := range state.Pins {
		recordedPins[pin.ID] = true
	}
	for id := range requiredPins {
		if !recordedPins[id] {
			closure = "OPEN"
			reasons = append(reasons, "dependency pin missing: "+id)
		}
	}
	for _, pin := range state.Pins {
		if !requiredPins[pin.ID] {
			closure = "OPEN"
			reasons = append(reasons, "dependency pin no longer declared: "+pin.ID)
		}
	}
	for _, entry := range validationcache.ComparePromisePins(state.Pins, current) {
		if entry.Status != validationcache.PromiseFresh {
			if closure != "OPEN" {
				closure = "REOPENED"
			}
			reasons = append(reasons, entry.ID+" is "+string(entry.Status))
		}
	}
	return closure, reasons
}

func promiseStateReason(reason string) string {
	if reason == "" {
		return ""
	}
	return " — " + reason
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
