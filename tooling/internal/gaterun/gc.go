package gaterun

import (
	"os"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/judgments"
	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/validationcache"
)

// CollectedJudgments reports what one collection pass removed.
type CollectedJudgments struct {
	Count int
	Bytes int64
}

// CollectJudgments deletes stale-protocol judgment records that no live
// reference can reach. The live set is the union of every published cache's
// GATE_JUDGMENTS bindings, the deployed standard's current-decision pointers,
// and every on-disk open run's planned bindings — including run states the
// strict loader rejects, and the shared task records those runs reach —
// expanded transitively through the record-reference chain. Records outside
// this closure whose review protocol differs from the deployed protocol can
// never pass judgments.Check again — a protocol mismatch fails closed and old
// in-progress runs must be replanned — so they are pure storage debt.
// Same-protocol superseded history is retained.
//
// A record's standard is the first component of its slot, so staleness is
// decided from where a record is filed, never from its body: collecting a
// whole retired standard costs directory reads, and an unreadable record filed
// under the current standard stays with the rest of that standard's history.
// A current-decision pointer confers liveness only under the standard that
// filed it: no reader asks a retired standard for a decision, so that decision
// is storage debt too, and Remove drops the pointer with its record. The caller
// must hold the repository mutation lock (gate-finalize's transaction).
func CollectJudgments(root string) (CollectedJudgments, error) {
	protocol, err := judgments.Protocol(root, GateVerify, TargetKindUnit)
	if err != nil {
		return CollectedJudgments{}, err
	}
	live := map[string]bool{}
	var queue []judgments.Reference
	seed := func(ref judgments.Reference) {
		if ref.ID == "" || live[ref.ID] {
			return
		}
		live[ref.ID] = true
		queue = append(queue, ref)
	}
	cacheRefs, err := validationcache.CacheJudgmentReferences(root)
	if err != nil {
		return CollectedJudgments{}, err
	}
	for _, ref := range cacheRefs {
		seed(ref)
	}
	currentRefs, err := judgments.Current(root, protocol)
	if err != nil {
		return CollectedJudgments{}, err
	}
	for _, ref := range currentRefs {
		seed(ref)
	}
	// The open-run closure is enumerated raw, not through ListRuns: a run
	// state the strict loader rejects (a retired protocol, an invalid shape)
	// is skipped by every listing, yet its run.json still binds records that
	// nothing else protects. "Skipped for display" must not become "skipped
	// for liveness".
	openRuns, err := openRunStates(root)
	if err != nil {
		return CollectedJudgments{}, err
	}
	for _, raw := range openRuns {
		// An open run's plan may consume its bound records at any moment,
		// including mid-finalize of another run; its bindings are live.
		for _, binding := range raw.Records {
			seed(binding.Reference)
		}
		for _, id := range raw.Tasks {
			task, err := readTask(root, id)
			if err != nil {
				if os.IsNotExist(err) {
					// The sweep severs a rejected run's task references by
					// removing the task file; a missing file carries no
					// binding to protect.
					continue
				}
				return CollectedJudgments{}, err
			}
			if task.Record != nil {
				seed(*task.Record)
			}
		}
	}
	for len(queue) > 0 {
		ref := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		record, err := judgments.Load(root, ref)
		if err != nil {
			continue
		}
		for _, dep := range record.References {
			seed(dep.Reference)
		}
	}
	all, err := judgments.List(root)
	if err != nil {
		return CollectedJudgments{}, err
	}
	var collected CollectedJudgments
	for _, ref := range all {
		if live[ref.ID] {
			continue
		}
		recordProtocol, err := judgments.SlotProtocol(ref.Slot)
		if err != nil {
			return CollectedJudgments{}, err
		}
		if recordProtocol == protocol {
			continue
		}
		bytes, err := judgments.Remove(root, ref)
		if err != nil {
			return CollectedJudgments{}, err
		}
		collected.Count++
		collected.Bytes += bytes
	}
	return collected, nil
}
