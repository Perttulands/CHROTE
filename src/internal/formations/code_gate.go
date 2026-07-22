package formations

import (
	"fmt"
	"sort"
	"strings"
)

// CodeGateEvaluator is a deterministic, in-process GateEvaluator for machine
// ("code") gates. It is the concrete pure code-Gate profile evaluator wired into
// the run engine so a defined machine gate returns a real pass/fail verdict
// instead of blocking with missing_gate_evaluator.
//
// It runs ONLY an explicit, operator-declared check profile with a validated
// non-secret parameter against the exact routed upstream output. It never spawns
// a process, opens the network, reads the host, or interprets the free-text gate
// criterion as an executable step (FORMATIONS.md rule 9). An undeclared or
// unknown check profile is a declaration error, never a silent pass and never a
// fallback to prose.
type CodeGateEvaluator struct{}

// NewCodeGateEvaluator constructs the default pure code-Gate evaluator. It holds
// no host authority: same input, same verdict, no side effects.
func NewCodeGateEvaluator() *CodeGateEvaluator {
	return &CodeGateEvaluator{}
}

// codeGateProfile is a pure function of the routed output text and the declared
// non-secret parameter. It returns the pass decision plus human-readable
// evidence recorded into the gate verdict.
type codeGateProfile func(output, value string) (pass bool, evidence string)

// codeGateProfiles is the registry of host-owned pure evaluator profiles. Each
// evaluates the declared upstream output; none has any process, network, or
// host-read authority. Keep additions pure and deterministic.
var codeGateProfiles = map[string]codeGateProfile{
	// output_contains passes when the routed output contains the declared token,
	// e.g. an upstream lint/test step that reports "LINT OK" on success.
	"output_contains": func(output, value string) (bool, string) {
		if strings.Contains(output, value) {
			return true, fmt.Sprintf("output contains %q", value)
		}
		return false, fmt.Sprintf("output does not contain %q", value)
	},
	// output_absent passes when the routed output does NOT contain the declared
	// token, e.g. a build/test step whose output must not mention "error".
	"output_absent": func(output, value string) (bool, string) {
		if strings.Contains(output, value) {
			return false, fmt.Sprintf("output unexpectedly contains %q", value)
		}
		return true, fmt.Sprintf("output does not contain %q", value)
	},
}

func knownCodeGateProfiles() []string {
	names := make([]string, 0, len(codeGateProfiles))
	for name := range codeGateProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// EvaluateGate applies the gate's explicitly-declared check profile to the exact
// routed upstream output and returns a deterministic pass/fail verdict with
// evidence. It returns an error (which the engine records as gate_evaluator_error
// and blocks on) when the gate declares no check, an unknown profile, or an empty
// parameter — machine gates require an explicit, operator-authored declaration.
func (c *CodeGateEvaluator) EvaluateGate(req GateEvaluation) (GateEvaluationResult, error) {
	check := strings.TrimSpace(req.Check)
	if check == "" {
		return GateEvaluationResult{}, fmt.Errorf(
			"gate %q is a machine gate with no explicit check profile; declare an operator-authored check (free-text criteria never execute)",
			req.GateID,
		)
	}
	profile, ok := codeGateProfiles[check]
	if !ok {
		return GateEvaluationResult{}, fmt.Errorf(
			"gate %q references unknown check profile %q; known profiles: %s",
			req.GateID, check, strings.Join(knownCodeGateProfiles(), ", "),
		)
	}
	if strings.TrimSpace(req.CheckValue) == "" {
		return GateEvaluationResult{}, fmt.Errorf(
			"gate %q check profile %q requires a non-empty checkValue parameter",
			req.GateID, check,
		)
	}

	pass, evidence := profile(req.Input.Text, req.CheckValue)
	verdict := "fail"
	if pass {
		verdict = "pass"
	}
	return GateEvaluationResult{
		Verdict: verdict,
		Reason:  fmt.Sprintf("check %s: %s", check, evidence),
		PerKind: codeGatePerKind(req.Kinds, verdict),
	}, nil
}

// codeGatePerKind records the machine verdict for every declared gate kind so
// the ledger keeps per-kind provenance even for multi-kind gates.
func codeGatePerKind(kinds []string, verdict string) map[string]string {
	perKind := make(map[string]string, len(kinds))
	for _, kind := range kinds {
		perKind[kind] = verdict
	}
	if len(perKind) == 0 {
		perKind["code"] = verdict
	}
	return perKind
}
