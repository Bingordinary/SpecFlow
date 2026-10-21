package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specpaths"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/specvalidation"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// promiseStateReport is the complete contract-and-evidence state of one unit
// layer. It is rendered by the `fresh` detail view.
type promiseStateReport struct {
	Unit          string
	Layer         string
	Promises      []specvalidation.Promise
	Own           map[string]string
	Coverage      specvalidation.CheckResult
	Graph         specvalidation.CheckResult
	State         *validationcache.PromiseStateFile
	StateRecorded bool
	Current       map[string]string
	CurrentIssues []string
	PinStates     []validationcache.PromiseState
	// Publication compares the recorded dependency pins with the published
	// stable contracts. A candidate may be DONE while it builds on another
	// candidate (current-layer scope), but it is not promotable until every
	// pinned provider promise is already published at the same fingerprint.
	Publication       []validationcache.PromiseState
	PublicationIssues []string
	Closure           string
	ClosureReasons    []string
	ValidateCheck     validationcache.CheckResult
	VerifyCheck       validationcache.CheckResult
	Done              bool
	DoneReasons       []string
}

// computePromiseState reads one unit layer and returns its contract closure
// and gate-evidence state. The promise graph is the layer's own: a candidate
// resolves providers in the current layer, a stable layer resolves providers
// in the published stable layer. A unit without a `## Promises` section is an
// error: the deployed unit shape always carries the promise layer.
func computePromiseState(absRoot, unitName, layer string) (*promiseStateReport, error) {
	graph, err := specvalidation.LoadPromiseGraphScope(absRoot, specvalidation.PromiseScopeForLayer(layer))
	if err != nil {
		return nil, err
	}
	stableGraph := graph
	if layer != "stable" {
		stableGraph, err = specvalidation.LoadPromiseGraphScope(absRoot, specvalidation.PromiseScopeStable)
		if err != nil {
			return nil, err
		}
	}
	return computePromiseStateWithGraph(absRoot, unitName, layer, graph, stableGraph)
}

func computePromiseStateWithGraph(absRoot, unitName, layer string, graph, stableGraph *specvalidation.PromiseGraph) (*promiseStateReport, error) {
	report := &promiseStateReport{Unit: unitName, Layer: layer, Own: map[string]string{}}
	relPath := specpaths.CandidateUnitSpecFileRef(unitName)
	if layer == "stable" {
		relPath = specpaths.StableUnitSpecFileRef(unitName)
	}
	data, err := os.ReadFile(filepath.Join(absRoot, filepath.FromSlash(relPath)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", relPath, err)
	}
	content := string(data)
	promises, parseIssues := specvalidation.ParsePromises(content)
	if len(parseIssues) > 0 {
		return nil, fmt.Errorf("promises: %s", strings.Join(parseIssues, "; "))
	}
	report.Promises = promises
	for _, promise := range promises {
		fingerprint, err := specvalidation.PromiseFingerprint(promise)
		if err != nil {
			return nil, err
		}
		report.Own[promise.ID] = fingerprint
	}
	report.Coverage = specvalidation.CheckPromiseCoverage(content, unitName)
	report.Graph = graph.Check(unitName)
	report.Current, report.CurrentIssues = graph.CurrentFingerprints()

	statePath, err := validationcache.PromiseStatePath(absRoot, unitName, layer)
	if err != nil {
		return nil, err
	}
	report.State, err = validationcache.ReadPromiseStateFile(statePath)
	if err != nil {
		return nil, err
	}
	if report.State == nil {
		report.State = &validationcache.PromiseStateFile{Unit: unitName, Layer: layer}
	} else {
		report.StateRecorded = true
		report.PinStates = validationcache.ComparePromisePins(report.State.Pins, report.Current)
	}
	// Publication is stricter than layer-local closure: a candidate may be
	// DONE while it builds on another candidate, but promote requires every
	// pinned provider promise to already exist in the published stable layer.
	if len(report.State.Pins) > 0 {
		if stableGraph == nil {
			stableGraph = graph
		}
		stable, issues := stableGraph.CurrentFingerprints()
		report.PublicationIssues = issues
		report.Publication = validationcache.ComparePromisePublication(report.State.Pins, stable)
	}
	report.Closure, report.ClosureReasons = promiseClosure(report.State, report.StateRecorded, report.Unit, report.Layer, report.Promises, report.Own, report.Current, report.Coverage, report.Graph)

	report.ValidateCheck, report.VerifyCheck, err = evidenceChecks(absRoot, unitName, layer)
	if err != nil {
		return nil, err
	}
	report.Done = report.Closure == "CLOSED" && report.StateRecorded && report.ValidateCheck.Fresh && report.VerifyCheck.Fresh
	report.DoneReasons = append(report.DoneReasons, report.ClosureReasons...)
	if !report.ValidateCheck.Fresh {
		report.DoneReasons = append(report.DoneReasons, "validate "+string(report.ValidateCheck.Category)+": "+report.ValidateCheck.Reason)
	}
	if !report.VerifyCheck.Fresh {
		report.DoneReasons = append(report.DoneReasons, "verify "+string(report.VerifyCheck.Category)+": "+report.VerifyCheck.Reason)
	}
	return report, nil
}
