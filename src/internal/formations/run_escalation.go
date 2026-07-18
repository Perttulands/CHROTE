package formations

import "strings"

type EscalationSentinel struct {
	RunID    string
	Severity string
	Reason   string
	NodeID   string
	GateID   string
	Blocks   bool
}

type OpenEscalation struct {
	RunID    string `json:"runId"`
	Seq      int    `json:"seq"`
	NodeID   string `json:"nodeId,omitempty"`
	GateID   string `json:"gateId,omitempty"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
	Source   string `json:"source"`
	Trigger  string `json:"trigger"`
	Blocks   bool   `json:"blocks"`
}

func ParseEscalationSentinel(captured, runID string) (EscalationSentinel, bool) {
	remaining := captured
	for {
		start := strings.Index(remaining, "<<<CHROTE-ESCALATE ")
		if start == -1 {
			return EscalationSentinel{}, false
		}
		remaining = remaining[start+len("<<<CHROTE-ESCALATE "):]
		end := strings.Index(remaining, ">>>")
		if end == -1 {
			return EscalationSentinel{}, false
		}
		fields := parseSentinelFieldsQuoted(remaining[:end])
		remaining = remaining[end+len(">>>"):]
		if fields["run-id"] != runID {
			continue
		}
		severity := fields["severity"]
		if severity == "" {
			severity = "needs-attention"
		}
		blocks := fields["blocks"] == "true" || severity == "stop"
		nodeID := fields["node"]
		if nodeID == "" {
			nodeID = fields["nodeId"]
		}
		return EscalationSentinel{
			RunID:    fields["run-id"],
			Severity: severity,
			Reason:   fields["reason"],
			NodeID:   nodeID,
			GateID:   fields["gateId"],
			Blocks:   blocks,
		}, true
	}
}

func (s *Store) RecordEscalationFromCapture(runID, nodeID, captured string) (bool, error) {
	if err := s.RequireRuntimeAuthority(); err != nil {
		return false, err
	}
	sentinel, ok := ParseEscalationSentinel(captured, runID)
	if !ok {
		return false, nil
	}
	if sentinel.NodeID != "" {
		nodeID = sentinel.NodeID
	}
	if err := s.AppendRunEvent(runID, RunEvent{
		Type:   RunEventEscalationRaised,
		NodeID: nodeID,
		GateID: sentinel.GateID,
		Data: map[string]any{
			"trigger":  "sentinel",
			"severity": sentinel.Severity,
			"reason":   sentinel.Reason,
			"source":   "agent",
			"nodeId":   nodeID,
			"gateId":   sentinel.GateID,
			"blocks":   sentinel.Blocks,
		},
	}); err != nil {
		return false, err
	}
	if sentinel.Blocks {
		if err := s.AppendRunEvent(runID, RunEvent{
			Type:   RunEventBlocked,
			NodeID: nodeID,
			GateID: sentinel.GateID,
			Data: map[string]any{
				"reason":         sentinel.Reason,
				"blockedNodeId":  nodeID,
				"blockedGateId":  sentinel.GateID,
				"resumeAllowed":  true,
				"resumePolicy":   "explicit",
				"openDispatches": []map[string]any{},
			},
		}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s *Store) ProjectOpenEscalations(runID string) ([]OpenEscalation, error) {
	events, err := s.ReadRunEvents(runID)
	if err != nil {
		return nil, err
	}
	open := make([]OpenEscalation, 0)
	for _, event := range events {
		if event.Type != RunEventEscalationRaised {
			continue
		}
		open = append(open, OpenEscalation{
			RunID:    runID,
			Seq:      event.Seq,
			NodeID:   event.NodeID,
			GateID:   event.GateID,
			Severity: stringFromEventData(event, "severity"),
			Reason:   stringFromEventData(event, "reason"),
			Source:   stringFromEventData(event, "source"),
			Trigger:  stringFromEventData(event, "trigger"),
			Blocks:   boolFromEventData(event, "blocks"),
		})
	}
	return open, nil
}

func parseSentinelFieldsQuoted(raw string) map[string]string {
	fields := map[string]string{}
	for i := 0; i < len(raw); {
		for i < len(raw) && raw[i] == ' ' {
			i++
		}
		start := i
		for i < len(raw) && raw[i] != '=' && raw[i] != ' ' {
			i++
		}
		if i >= len(raw) || raw[i] != '=' {
			for i < len(raw) && raw[i] != ' ' {
				i++
			}
			continue
		}
		key := raw[start:i]
		i++
		if i >= len(raw) {
			fields[key] = ""
			break
		}
		var value string
		if raw[i] == '\'' || raw[i] == '"' {
			quote := raw[i]
			i++
			valueStart := i
			for i < len(raw) && raw[i] != quote {
				i++
			}
			value = raw[valueStart:i]
			if i < len(raw) {
				i++
			}
		} else {
			valueStart := i
			for i < len(raw) && raw[i] != ' ' {
				i++
			}
			value = raw[valueStart:i]
		}
		fields[key] = value
	}
	return fields
}
