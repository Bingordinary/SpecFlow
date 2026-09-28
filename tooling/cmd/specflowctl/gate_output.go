package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Bingordinary/SpecFlow/specflow/tooling/internal/gaterun"
)

type gatePacketView struct {
	PacketID      string   `json:"packet_id"`
	Kind          string   `json:"kind"`
	CheckKeys     []string `json:"check_keys"`
	DependsOn     []string `json:"depends_on"`
	Status        string   `json:"status"`
	Attempts      int      `json:"attempts"`
	LastRejection string   `json:"last_rejection,omitempty"`
}

type gateRunView struct {
	SchemaVersion    int                       `json:"schema_version"`
	RunID            string                    `json:"run_id"`
	Gate             string                    `json:"gate"`
	TargetKind       string                    `json:"target_kind"`
	TargetName       string                    `json:"target_name"`
	Target           string                    `json:"target"`
	Mode             string                    `json:"mode"`
	Status           string                    `json:"status"`
	Packets          []gatePacketView          `json:"packets"`
	ReadyPacketIDs   []string                  `json:"ready_packet_ids"`
	NextAction       string                    `json:"next_action"`
	CarriedKeys      []string                  `json:"carried_keys"`
	DeferredFindings []gaterun.DeferredFinding `json:"deferred_findings"`
	Notices          []string                  `json:"notices"`
}

func writeGateJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func gateRunSnapshot(root string, run *gaterun.Run) (gateRunView, error) {
	view := gateRunView{
		SchemaVersion: 1, RunID: run.RunID, Gate: run.Gate,
		TargetKind: run.TargetKind, TargetName: run.TargetName, Target: run.Target,
		Mode: run.Mode, Status: run.Status,
		Packets: []gatePacketView{}, ReadyPacketIDs: []string{},
		CarriedKeys:      append([]string{}, run.CarriedKeys...),
		DeferredFindings: append([]gaterun.DeferredFinding{}, run.DeferredFindings...),
		Notices:          append([]string{}, run.Notices...),
	}
	states := make(map[string]*gaterun.PacketState, len(run.Packets))
	for _, packet := range run.Packets {
		state, err := gaterun.LoadPacketState(root, run, packet.PacketID)
		if err != nil {
			return gateRunView{}, err
		}
		states[packet.PacketID] = state
		entry := gatePacketView{
			PacketID: packet.PacketID, Kind: packet.Kind,
			CheckKeys: append([]string{}, packet.CheckKeys...),
			DependsOn: append([]string{}, packet.DependsOn...),
			Status:    state.Status, Attempts: len(state.Attempts),
		}
		if len(state.Attempts) > 0 {
			entry.LastRejection = state.Attempts[len(state.Attempts)-1].RejectionReason
		}
		view.Packets = append(view.Packets, entry)
	}
	if run.Status != gaterun.StatusOpen {
		view.NextAction = "none"
		return view, nil
	}
	for _, packet := range run.Packets {
		state := states[packet.PacketID]
		if state.Status != gaterun.PacketPending && state.Status != gaterun.PacketRejected {
			continue
		}
		ready := true
		for _, dep := range packet.DependsOn {
			depState := states[dep]
			if depState == nil {
				return gateRunView{}, fmt.Errorf("packet %q has missing dependency %q", packet.PacketID, dep)
			}
			if depState.Status != gaterun.PacketAccepted && depState.Status != gaterun.PacketNotRequired {
				ready = false
				break
			}
		}
		if ready {
			view.ReadyPacketIDs = append(view.ReadyPacketIDs, packet.PacketID)
		}
	}
	if len(view.ReadyPacketIDs) > 0 {
		view.NextAction = "execute"
	} else {
		view.NextAction = "finalize"
	}
	return view, nil
}
