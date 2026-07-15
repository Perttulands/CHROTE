package formations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path"
	"strconv"
	"strings"
)

const maxScorecardBytes = 64 * 1024

type ScorecardGateEvaluator struct{}

type gateScorecard struct {
	Schema           int               `json:"schema"`
	ClaimedComposite *float64          `json:"claimedComposite,omitempty"`
	ArtifactRef      string            `json:"artifactRef,omitempty"`
	Summary          string            `json:"summary,omitempty"`
	Reviews          []scorecardReview `json:"reviews"`
}

type scorecardReview struct {
	Reviewer        string   `json:"reviewer"`
	Score           float64  `json:"score"`
	Evidence        []string `json:"evidence"`
	MustFix         []string `json:"mustFix,omitempty"`
	Strengths       []string `json:"strengths,omitempty"`
	Recommendations []string `json:"recommendations,omitempty"`
}

func (ScorecardGateEvaluator) EvaluateGate(req GateEvaluation) (GateEvaluationResult, error) {
	if !scorecardGateKinds(req.Kinds) {
		return scorecardGateFail(req.Kinds, "scorecard gate requires exactly the scorecard kind"), nil
	}
	weights, err := validateScorecardPolicy(req)
	if err != nil {
		return scorecardGateFail(req.Kinds, err.Error()), nil
	}
	text := strings.TrimSpace(req.Input.Text)
	if text == "" {
		return scorecardGateFail(req.Kinds, "invalid scorecard: routed input text is empty"), nil
	}
	if len(text) > maxScorecardBytes {
		return scorecardGateFail(req.Kinds, fmt.Sprintf("invalid scorecard: input exceeds %d bytes", maxScorecardBytes)), nil
	}
	var scorecard gateScorecard
	decoder := json.NewDecoder(bytes.NewBufferString(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scorecard); err != nil {
		return scorecardGateFail(req.Kinds, "invalid scorecard: "+err.Error()), nil
	}
	if err := ensureScorecardEOF(decoder); err != nil {
		return scorecardGateFail(req.Kinds, "invalid scorecard: "+err.Error()), nil
	}
	if scorecard.Schema != 1 {
		return scorecardGateFail(req.Kinds, fmt.Sprintf("invalid scorecard: unsupported schema %d", scorecard.Schema)), nil
	}
	scorecardArtifact, err := normalizeScorecardArtifactRef(scorecard.ArtifactRef)
	if err != nil {
		return scorecardGateFail(req.Kinds, "invalid scorecard: "+err.Error()), nil
	}
	routedArtifact, err := normalizeScorecardArtifactRef(req.Input.ArtifactRef)
	if err != nil {
		return scorecardGateFail(req.Kinds, "invalid routed artifact: "+err.Error()), nil
	}
	if scorecardArtifact != routedArtifact {
		return scorecardGateFail(req.Kinds, fmt.Sprintf("invalid scorecard: artifactRef %q does not match routed artifact %q", scorecardArtifact, routedArtifact)), nil
	}
	if len(scorecard.Reviews) == 0 {
		return scorecardGateFail(req.Kinds, "invalid scorecard: reviews are required"), nil
	}

	seen := map[string]bool{}
	weighted := 0.0
	mustFix := 0
	for index, review := range scorecard.Reviews {
		reviewer := strings.TrimSpace(review.Reviewer)
		weight, expected := weights[reviewer]
		if !expected {
			return scorecardGateFail(req.Kinds, fmt.Sprintf("invalid scorecard: unexpected reviewer %q", reviewer)), nil
		}
		if seen[reviewer] {
			return scorecardGateFail(req.Kinds, fmt.Sprintf("invalid scorecard: duplicate reviewer %q", reviewer)), nil
		}
		seen[reviewer] = true
		if math.IsNaN(review.Score) || math.IsInf(review.Score, 0) || review.Score < 0 || review.Score > 10 {
			return scorecardGateFail(req.Kinds, fmt.Sprintf("invalid scorecard: review %d score must be between 0 and 10", index)), nil
		}
		if len(review.Evidence) == 0 {
			return scorecardGateFail(req.Kinds, fmt.Sprintf("invalid scorecard: reviewer %q must cite evidence", reviewer)), nil
		}
		for _, evidence := range review.Evidence {
			if strings.TrimSpace(evidence) == "" {
				return scorecardGateFail(req.Kinds, fmt.Sprintf("invalid scorecard: reviewer %q contains empty evidence", reviewer)), nil
			}
		}
		for _, finding := range review.MustFix {
			if strings.TrimSpace(finding) == "" {
				return scorecardGateFail(req.Kinds, fmt.Sprintf("invalid scorecard: reviewer %q contains an empty must-fix finding", reviewer)), nil
			}
			mustFix++
		}
		weighted += review.Score * weight
	}
	for reviewer := range weights {
		if !seen[reviewer] {
			return scorecardGateFail(req.Kinds, fmt.Sprintf("invalid scorecard: missing required reviewer %q", reviewer)), nil
		}
	}

	reason := fmt.Sprintf("authoritative score %.2f/10 threshold %.2f; mustFix=%d", weighted, req.ScoreThreshold, mustFix)
	if weighted < req.ScoreThreshold {
		return scorecardGateFail(req.Kinds, reason+"; below threshold"), nil
	}
	if req.RequireNoMustFix && mustFix > 0 {
		return scorecardGateFail(req.Kinds, reason+"; unresolved must-fix findings"), nil
	}
	return GateEvaluationResult{
		Verdict: "pass",
		Reason:  truncateGateReason(redactLedgerText(reason)),
		PerKind: map[string]string{"scorecard": "pass"},
	}, nil
}

func scorecardGateKinds(kinds []string) bool {
	return len(kinds) == 1 && strings.TrimSpace(kinds[0]) == "scorecard"
}

func normalizeScorecardArtifactRef(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return "", fmt.Errorf("artifactRef is required")
	}
	cleaned := path.Clean(value)
	first := strings.SplitN(cleaned, "/", 2)[0]
	if strings.HasPrefix(cleaned, "/") || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(first, ":") {
		return "", fmt.Errorf("artifactRef must be a workspace-relative path")
	}
	return cleaned, nil
}

func validateScorecardPolicy(req GateEvaluation) (map[string]float64, error) {
	if req.ScoreThreshold <= 0 || req.ScoreThreshold > 10 || math.IsNaN(req.ScoreThreshold) || math.IsInf(req.ScoreThreshold, 0) {
		return nil, fmt.Errorf("invalid scorecard policy: threshold must be greater than 0 and at most 10")
	}
	if len(req.RequiredReviewers) == 0 {
		return nil, fmt.Errorf("invalid scorecard policy: required reviewers are missing")
	}
	required := map[string]bool{}
	for _, raw := range req.RequiredReviewers {
		reviewer := strings.TrimSpace(raw)
		if reviewer == "" {
			return nil, fmt.Errorf("invalid scorecard policy: required reviewer is empty")
		}
		if required[reviewer] {
			return nil, fmt.Errorf("invalid scorecard policy: required reviewer %q is duplicated", reviewer)
		}
		required[reviewer] = true
	}
	weights := map[string]float64{}
	for _, raw := range req.ReviewerWeights {
		name, value, ok := strings.Cut(raw, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("invalid scorecard policy: reviewer weight %q must use reviewer=value", raw)
		}
		if !required[name] {
			return nil, fmt.Errorf("invalid scorecard policy: weight names unexpected reviewer %q", name)
		}
		if _, exists := weights[name]; exists {
			return nil, fmt.Errorf("invalid scorecard policy: weight for reviewer %q is duplicated", name)
		}
		weight, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || weight <= 0 || math.IsNaN(weight) || math.IsInf(weight, 0) {
			return nil, fmt.Errorf("invalid scorecard policy: reviewer %q has invalid weight", name)
		}
		weights[name] = weight
	}
	if len(weights) != len(required) {
		return nil, fmt.Errorf("invalid scorecard policy: every required reviewer needs one weight")
	}
	sum := 0.0
	for _, weight := range weights {
		sum += weight
	}
	if math.Abs(sum-1.0) > 0.000001 {
		return nil, fmt.Errorf("invalid scorecard policy: reviewer weights must sum to 1 (got %.6f)", sum)
	}
	return weights, nil
}

func scorecardGateFail(kinds []string, reason string) GateEvaluationResult {
	perKind := make(map[string]string, len(kinds))
	for _, kind := range kinds {
		perKind[kind] = "fail"
	}
	return GateEvaluationResult{
		Verdict: "fail",
		Reason:  truncateGateReason(redactLedgerText(reason)),
		PerKind: perKind,
	}
}

func ensureScorecardEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("multiple JSON values")
}
