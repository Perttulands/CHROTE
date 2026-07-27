package formations

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrInvalidCodeGateProfile = errors.New(FindingInvalidCodeGateProfile)

const (
	CodeGateExecutionClassInProcess       = "in_process"
	CodeGateEffectPolicyNone              = "none"
	CodeGateResultEncoding                = "decision-result-jcs-v1"
	codeGateEvaluatorImplementationSHA256 = "c1b0017bd2707a31c35bc85c95d01f2bca9330ac592ff543867a5cc47346be11"
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

type codeGateOperation string

const (
	codeGateOperationContains codeGateOperation = "string_contains"
	codeGateOperationAbsent   codeGateOperation = "string_absent"
)

type codeGateProfileKey struct {
	id      string
	version string
}

// CodeGateProfileDescriptor is the immutable registry contract presented to
// authors and frozen into each run. Only certified total, deterministic,
// effect-free in-process profiles are admissible.
type CodeGateProfileDescriptor struct {
	ProfileID               string `json:"profileId"`
	ProfileVersion          string `json:"profileVersion"`
	DisplayName             string `json:"displayName"`
	ParameterName           string `json:"parameterName"`
	ParameterLabel          string `json:"parameterLabel"`
	ExecutionClass          string `json:"executionClass"`
	EffectPolicy            string `json:"effectPolicy"`
	Deterministic           bool   `json:"deterministic"`
	CertifiedTotal          bool   `json:"certifiedTotal"`
	MaxInputBytes           int    `json:"maxInputBytes"`
	MaxResultBytes          int    `json:"maxResultBytes"`
	MaxOperations           int    `json:"maxOperations"`
	ResultEncoding          string `json:"resultEncoding"`
	ProfileSHA256           string `json:"profileSha256"`
	EvaluatorBundleSHA256   string `json:"evaluatorBundleSha256"`
	PolicySHA256            string `json:"policySha256"`
	DeterminismPolicySHA256 string `json:"determinismPolicySha256"`
}

type codeGateProfileDefinition struct {
	descriptor CodeGateProfileDescriptor
	operation  codeGateOperation
}

func newCodeGateProfileDefinition(id, version, displayName, parameterLabel string, operation codeGateOperation) codeGateProfileDefinition {
	const (
		maxInputBytes  = 256 * 1024
		maxResultBytes = 8 * 1024
		maxOperations  = 512 * 1024
	)
	profileManifest := strings.Join([]string{
		"code-gate-profile-v1",
		"id=" + id,
		"version=" + version,
		"parameter=value:string:nonempty",
		"execution=" + CodeGateExecutionClassInProcess,
		"effects=" + CodeGateEffectPolicyNone,
		"deterministic=true",
		"certifiedTotal=true",
		fmt.Sprintf("maxInputBytes=%d", maxInputBytes),
		fmt.Sprintf("maxResultBytes=%d", maxResultBytes),
		fmt.Sprintf("maxOperations=%d", maxOperations),
		"resultEncoding=" + CodeGateResultEncoding,
		"evaluatorImplementationSha256=" + codeGateEvaluatorImplementationSHA256,
		"operation=" + string(operation),
	}, "\n")
	policyManifest := strings.Join([]string{
		"code-gate-policy-v1",
		"execution=" + CodeGateExecutionClassInProcess,
		"effects=" + CodeGateEffectPolicyNone,
		fmt.Sprintf("maxInputBytes=%d", maxInputBytes),
		fmt.Sprintf("maxResultBytes=%d", maxResultBytes),
		fmt.Sprintf("maxOperations=%d", maxOperations),
	}, "\n")
	determinismManifest := strings.Join([]string{
		"code-gate-determinism-v1",
		"deterministic=true",
		"certifiedTotal=true",
		"resultEncoding=" + CodeGateResultEncoding,
	}, "\n")
	return codeGateProfileDefinition{
		descriptor: CodeGateProfileDescriptor{
			ProfileID:               id,
			ProfileVersion:          version,
			DisplayName:             displayName,
			ParameterName:           "value",
			ParameterLabel:          parameterLabel,
			ExecutionClass:          CodeGateExecutionClassInProcess,
			EffectPolicy:            CodeGateEffectPolicyNone,
			Deterministic:           true,
			CertifiedTotal:          true,
			MaxInputBytes:           maxInputBytes,
			MaxResultBytes:          maxResultBytes,
			MaxOperations:           maxOperations,
			ResultEncoding:          CodeGateResultEncoding,
			ProfileSHA256:           codeGateSHA256(profileManifest),
			EvaluatorBundleSHA256:   codeGateSHA256("code-gate-evaluator-bundle-v1\nimplementationSha256=" + codeGateEvaluatorImplementationSHA256 + "\noperation=" + string(operation)),
			PolicySHA256:            codeGateSHA256(policyManifest),
			DeterminismPolicySHA256: codeGateSHA256(determinismManifest),
		},
		operation: operation,
	}
}

func codeGateSHA256(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum)
}

func codeGateParametersCanonical(value string) string {
	var canonical bytes.Buffer
	canonical.WriteString(`{"value":`)
	writeRuntimeCanonicalJSONString(&canonical, value)
	canonical.WriteByte('}')
	return canonical.String()
}

var outputContainsCodeGateProfile = newCodeGateProfileDefinition(
	"output_contains",
	"1",
	"Output contains value",
	"Required text",
	codeGateOperationContains,
)

var outputAbsentCodeGateProfile = newCodeGateProfileDefinition(
	"output_absent",
	"1",
	"Output excludes value",
	"Forbidden text",
	codeGateOperationAbsent,
)

// codeGateProfiles is the registry of host-owned pure evaluator profiles. Each
// evaluates the declared upstream output; none has any process, network, or
// host-read authority. Keep additions pure and deterministic.
var codeGateProfiles = map[codeGateProfileKey]codeGateProfileDefinition{
	// output_contains passes when the routed output contains the declared token,
	// e.g. an upstream lint/test step that reports "LINT OK" on success.
	{id: "output_contains", version: "1"}: outputContainsCodeGateProfile,
	// output_absent passes when the routed output does NOT contain the declared
	// token, e.g. a build/test step whose output must not mention "error".
	{id: "output_absent", version: "1"}: outputAbsentCodeGateProfile,
}

// ListCodeGateProfileDescriptors returns a stable authoring view of every
// registered exact profile tuple.
func ListCodeGateProfileDescriptors() []CodeGateProfileDescriptor {
	descriptors := make([]CodeGateProfileDescriptor, 0, len(codeGateProfiles))
	for _, profile := range codeGateProfiles {
		descriptors = append(descriptors, profile.descriptor)
	}
	sort.Slice(descriptors, func(i, j int) bool {
		if descriptors[i].ProfileID == descriptors[j].ProfileID {
			return descriptors[i].ProfileVersion < descriptors[j].ProfileVersion
		}
		return descriptors[i].ProfileID < descriptors[j].ProfileID
	})
	return descriptors
}

// LookupCodeGateProfileDescriptor resolves one exact immutable profile tuple.
func LookupCodeGateProfileDescriptor(id, version string) (CodeGateProfileDescriptor, bool) {
	profile, ok := codeGateProfiles[codeGateProfileKey{
		id:      strings.TrimSpace(id),
		version: strings.TrimSpace(version),
	}]
	return profile.descriptor, ok
}

func validateCodeGateProfileDescriptor(descriptor CodeGateProfileDescriptor) error {
	switch {
	case strings.TrimSpace(descriptor.ProfileID) == "" || strings.TrimSpace(descriptor.ProfileVersion) == "":
		return fmt.Errorf("profile identity is incomplete")
	case descriptor.ExecutionClass != CodeGateExecutionClassInProcess:
		return fmt.Errorf("profile %q@%q execution class %q is not admissible", descriptor.ProfileID, descriptor.ProfileVersion, descriptor.ExecutionClass)
	case descriptor.EffectPolicy != CodeGateEffectPolicyNone:
		return fmt.Errorf("profile %q@%q effect policy %q is not admissible", descriptor.ProfileID, descriptor.ProfileVersion, descriptor.EffectPolicy)
	case !descriptor.Deterministic:
		return fmt.Errorf("profile %q@%q is not deterministic", descriptor.ProfileID, descriptor.ProfileVersion)
	case !descriptor.CertifiedTotal:
		return fmt.Errorf("profile %q@%q is not certified total", descriptor.ProfileID, descriptor.ProfileVersion)
	case descriptor.MaxInputBytes <= 0 || descriptor.MaxResultBytes <= 0 || descriptor.MaxOperations <= 0:
		return fmt.Errorf("profile %q@%q has invalid evaluator limits", descriptor.ProfileID, descriptor.ProfileVersion)
	case descriptor.ResultEncoding != CodeGateResultEncoding:
		return fmt.Errorf("profile %q@%q result encoding %q is not admissible", descriptor.ProfileID, descriptor.ProfileVersion, descriptor.ResultEncoding)
	case descriptor.ProfileSHA256 == "" || descriptor.EvaluatorBundleSHA256 == "" || descriptor.PolicySHA256 == "" || descriptor.DeterminismPolicySHA256 == "":
		return fmt.Errorf("profile %q@%q is missing frozen content hashes", descriptor.ProfileID, descriptor.ProfileVersion)
	default:
		return nil
	}
}

func knownCodeGateProfiles() []string {
	names := make([]string, 0, len(codeGateProfiles))
	for key := range codeGateProfiles {
		names = append(names, key.id+"@"+key.version)
	}
	sort.Strings(names)
	return names
}

func codeGateDefinitionIsRoutable(gate GateNode) bool {
	if !hasGateKind(gate.Kinds, "code") {
		return true
	}
	key := codeGateProfileKey{
		id:      strings.TrimSpace(gate.Check),
		version: strings.TrimSpace(gate.CheckVersion),
	}
	profile, ok := codeGateProfiles[key]
	if !ok || validateCodeGateProfileDescriptor(profile.descriptor) != nil {
		return false
	}
	return strings.TrimSpace(gate.CheckValue) != ""
}

// BEGIN CODE GATE EVALUATOR BUNDLE
func evaluateCodeGateOperation(operation codeGateOperation, output, value string) (bool, string, error) {
	switch operation {
	case codeGateOperationContains:
		if strings.Contains(output, value) {
			return true, fmt.Sprintf("output contains %q", value), nil
		}
		return false, fmt.Sprintf("output does not contain %q", value), nil
	case codeGateOperationAbsent:
		if strings.Contains(output, value) {
			return false, fmt.Sprintf("output unexpectedly contains %q", value), nil
		}
		return true, fmt.Sprintf("output does not contain %q", value), nil
	default:
		return false, "", fmt.Errorf("unknown code Gate operation %q", operation)
	}
}

// END CODE GATE EVALUATOR BUNDLE

// EvaluateGate applies the gate's explicitly-declared check profile to the exact
// routed upstream output and returns a deterministic pass/fail verdict with
// evidence. It returns an error (which the engine records as gate_evaluator_error
// and blocks on) when the gate declares no check, an unknown profile, or an empty
// parameter — machine gates require an explicit, operator-authored declaration.
func (c *CodeGateEvaluator) EvaluateGate(req GateEvaluation) (GateEvaluationResult, error) {
	check := strings.TrimSpace(req.Check)
	checkVersion := strings.TrimSpace(req.CheckVersion)
	if check == "" {
		return GateEvaluationResult{}, fmt.Errorf(
			"gate %q is a machine gate with no explicit check profile; declare an operator-authored check (free-text criteria never execute)",
			req.GateID,
		)
	}
	profile, ok := codeGateProfiles[codeGateProfileKey{id: check, version: checkVersion}]
	if !ok {
		return GateEvaluationResult{}, fmt.Errorf(
			"gate %q references unknown check profile tuple %q@%q; known profiles: %s",
			req.GateID, check, checkVersion, strings.Join(knownCodeGateProfiles(), ", "),
		)
	}
	if err := validateRunGateBinding(req, profile.descriptor); err != nil {
		return GateEvaluationResult{}, err
	}
	if strings.TrimSpace(req.CheckValue) == "" {
		return GateEvaluationResult{}, fmt.Errorf(
			"gate %q check profile %q requires a non-empty checkValue parameter",
			req.GateID, check,
		)
	}
	inputBytes := len([]byte(req.Input.Text))
	if inputBytes > req.Binding.MaxInputBytes {
		return GateEvaluationResult{}, fmt.Errorf(
			"gate %q evaluator exhausted maxInputBytes: %d > %d",
			req.GateID, inputBytes, req.Binding.MaxInputBytes,
		)
	}
	operations := uint64(inputBytes) * uint64(max(1, len([]byte(req.CheckValue))))
	if operations > uint64(req.Binding.MaxOperations) {
		return GateEvaluationResult{}, fmt.Errorf(
			"gate %q evaluator exhausted maxOperations: %d > %d",
			req.GateID, operations, req.Binding.MaxOperations,
		)
	}

	pass, evidence, err := evaluateCodeGateOperation(profile.operation, req.Input.Text, req.CheckValue)
	if err != nil {
		return GateEvaluationResult{}, fmt.Errorf("gate %q evaluator profile is invalid: %w", req.GateID, err)
	}
	verdict := "fail"
	if pass {
		verdict = "pass"
	}
	reason := fmt.Sprintf("check %s: %s", check, evidence)
	evidenceRefs := []GateEvidenceRef{{Kind: "text", Text: reason}}
	canonicalResult, err := canonicalCodeGateResult(verdict, reason, evidenceRefs)
	if err != nil {
		return GateEvaluationResult{}, fmt.Errorf("gate %q canonical result: %w", req.GateID, err)
	}
	if len([]byte(canonicalResult)) > req.Binding.MaxResultBytes {
		return GateEvaluationResult{}, fmt.Errorf(
			"gate %q evaluator exhausted maxResultBytes: %d > %d",
			req.GateID, len([]byte(canonicalResult)), req.Binding.MaxResultBytes,
		)
	}
	return GateEvaluationResult{
		Verdict:         verdict,
		Reason:          reason,
		Evidence:        evidenceRefs,
		PerKind:         codeGatePerKind(req.Kinds, verdict),
		ResultEncoding:  CodeGateResultEncoding,
		ResultSHA256:    codeGateSHA256(canonicalResult),
		CanonicalResult: canonicalResult,
		GateBindingID:   req.Binding.GateBindingID,
	}, nil
}

func canonicalCodeGateResult(verdict, reason string, evidence []GateEvidenceRef) (string, error) {
	var canonical bytes.Buffer
	canonical.WriteString(`{"evidence":[`)
	for index, reference := range evidence {
		if index > 0 {
			canonical.WriteByte(',')
		}
		canonical.WriteString(`{"kind":`)
		writeRuntimeCanonicalJSONString(&canonical, reference.Kind)
		if reference.Text != "" {
			canonical.WriteString(`,"text":`)
			writeRuntimeCanonicalJSONString(&canonical, reference.Text)
		}
		canonical.WriteByte('}')
	}
	canonical.WriteString(`],"reason":`)
	writeRuntimeCanonicalJSONString(&canonical, reason)
	canonical.WriteString(`,"verdict":`)
	writeRuntimeCanonicalJSONString(&canonical, verdict)
	canonical.WriteByte('}')
	return canonical.String(), nil
}

func validateRunGateBinding(req GateEvaluation, descriptor CodeGateProfileDescriptor) error {
	binding := req.Binding
	if binding == nil {
		return fmt.Errorf("gate %q has no frozen evaluator binding", req.GateID)
	}
	expectedBindingID := "gbd_" + codeGateSHA256(req.RunID + "\x00" + req.GateID)[:24]
	expectedParametersSHA256 := codeGateSHA256(codeGateParametersCanonical(req.CheckValue))
	switch {
	case binding.GateBindingID != expectedBindingID || binding.GateID != req.GateID:
		return fmt.Errorf("gate %q frozen evaluator binding identity mismatch", req.GateID)
	case binding.ProfileID != descriptor.ProfileID || binding.ProfileVersion != descriptor.ProfileVersion:
		return fmt.Errorf("gate %q frozen evaluator profile tuple mismatch", req.GateID)
	case binding.ProfileSHA256 != descriptor.ProfileSHA256:
		return fmt.Errorf("gate %q frozen evaluator profile content mismatch", req.GateID)
	case binding.EvaluatorBundleSHA256 != descriptor.EvaluatorBundleSHA256:
		return fmt.Errorf("gate %q frozen evaluator bundle mismatch", req.GateID)
	case len(binding.Parameters) != 1 || binding.Parameters["value"] != req.CheckValue || binding.ParametersSHA256 != expectedParametersSHA256:
		return fmt.Errorf("gate %q frozen evaluator parameters mismatch", req.GateID)
	case binding.PolicySHA256 != descriptor.PolicySHA256:
		return fmt.Errorf("gate %q frozen evaluator policy mismatch", req.GateID)
	case binding.DeterminismPolicySHA256 != descriptor.DeterminismPolicySHA256:
		return fmt.Errorf("gate %q frozen evaluator determinism policy mismatch", req.GateID)
	case binding.MaxInputBytes != descriptor.MaxInputBytes ||
		binding.MaxResultBytes != descriptor.MaxResultBytes ||
		binding.MaxOperations != descriptor.MaxOperations:
		return fmt.Errorf("gate %q frozen evaluator limits mismatch", req.GateID)
	case binding.ResultEncoding != descriptor.ResultEncoding:
		return fmt.Errorf("gate %q frozen evaluator result encoding mismatch", req.GateID)
	default:
		return nil
	}
}

// codeGatePerKind records only the completed machine kind. Later Gate kinds are
// evaluated independently by the engine's fixed code -> formation -> human
// sequence.
func codeGatePerKind(kinds []string, verdict string) map[string]string {
	return map[string]string{"code": verdict}
}
