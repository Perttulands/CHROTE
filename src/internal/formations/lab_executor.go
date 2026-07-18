package formations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const defaultLabOutputCapBytes = 8192

type RunExecutionError struct {
	Code       string
	Message    string
	Boundary   string
	Cause      error
	NodeID     string
	SlotID     string
	DispatchID string
}

func (e *RunExecutionError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *RunExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

type LabExecutorConfig struct {
	Harnesses      []string
	Cwd            string
	Roots          []string
	OutputCapBytes int
}

type LabFormationExecutor struct {
	store    *Store
	personas *PersonaStore
	config   LabExecutorConfig
}

func NewConfiguredFormationExecutorFromEnv(store *Store, personas *PersonaStore, boundary string) FormationExecutor {
	labConfig := LabExecutorConfigFromEnv()
	if len(labConfig.Harnesses) != 0 {
		return NewLabFormationExecutor(store, personas, labConfig)
	}
	tmuxConfig := TmuxExecutorConfigFromEnv()
	if len(tmuxConfig.Harnesses) != 0 {
		return NewTmuxFormationExecutor(store, personas, tmuxConfig)
	}
	return NewUnavailableFormationExecutor(boundary)
}

func LabExecutorConfigFromEnv() LabExecutorConfig {
	capBytes := defaultLabOutputCapBytes
	if raw := strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_LAB_OUTPUT_CAP_BYTES")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			capBytes = parsed
		}
	}
	return LabExecutorConfig{
		Harnesses:      splitLabCSV(os.Getenv("CHROTE_FORMATIONS_LAB_HARNESSES")),
		Cwd:            strings.TrimSpace(os.Getenv("CHROTE_FORMATIONS_LAB_CWD")),
		Roots:          splitLabCSV(os.Getenv("CHROTE_FORMATIONS_LAB_ROOTS")),
		OutputCapBytes: capBytes,
	}
}

func NewLabFormationExecutor(store *Store, personas *PersonaStore, config LabExecutorConfig) *LabFormationExecutor {
	if config.OutputCapBytes <= 0 {
		config.OutputCapBytes = defaultLabOutputCapBytes
	}
	return &LabFormationExecutor{store: store, personas: personas, config: config}
}

func (e *LabFormationExecutor) ExecuteFormation(req FormationExecution) (FormationExecutionResult, error) {
	if e == nil || e.store == nil {
		return FormationExecutionResult{}, runExecutionError("missing_executor", "lab executor store is not configured", "executor", ErrRunExecutorUnavailable)
	}
	if err := e.store.RequireRuntimeAuthority(); err != nil {
		return FormationExecutionResult{}, err
	}
	if err := e.validateConfiguredBoundary(); err != nil {
		return FormationExecutionResult{}, err
	}
	if len(req.Formation.Slots) == 0 {
		return FormationExecutionResult{}, runExecutionError("missing_slot", fmt.Sprintf("formation %q has no slots to dispatch", req.NodeID), "executor", nil)
	}

	allowed := e.allowedHarnesses()
	dispatcher := NewSlotDispatcher(e.store, nil)
	outputs := make([]string, 0, len(req.Formation.Slots))
	for _, slot := range req.Formation.Slots {
		if slot.AgentID == "" {
			return FormationExecutionResult{}, runExecutionError("missing_agent", fmt.Sprintf("slot %q is not staffed", slot.ID), "executor", nil)
		}
		if e.personas == nil {
			return FormationExecutionResult{}, runExecutionError("missing_persona_store", "persona store is not configured", "executor", nil)
		}
		card, err := e.personas.ReadPersona(slot.AgentID)
		if err != nil {
			return FormationExecutionResult{}, runExecutionError("missing_persona", fmt.Sprintf("persona %q could not be resolved", slot.AgentID), "executor", err)
		}
		variant, err := card.SelectHarnessVariant(slot.Harness)
		if err != nil {
			code := "missing_harness"
			if errors.Is(err, ErrAmbiguousAgentBinding) {
				code = "ambiguous_harness"
			}
			return FormationExecutionResult{}, runExecutionError(code, err.Error(), "executor", err)
		}
		if !allowed[variant.ID] {
			return FormationExecutionResult{}, runExecutionError("unconfigured_harness", fmt.Sprintf("lab executor is not configured for harness %q", variant.ID), "executor", nil)
		}
		if variant.SessionStem == "" {
			return FormationExecutionResult{}, runExecutionError("missing_session", fmt.Sprintf("agent %q harness %q has no session stem", card.ID, variant.ID), "executor", nil)
		}

		prompt := e.renderPrompt(req, slot, *card, variant)
		lease, err := dispatcher.DispatchSlot(req.RunID, SlotDispatchRequest{
			NodeID:      req.NodeID,
			SlotID:      slot.ID,
			AgentID:     card.ID,
			Harness:     variant.ID,
			SessionStem: variant.SessionStem,
			SessionRef:  "lab:" + variant.SessionStem,
			Prompt:      prompt,
			Attempt:     req.Attempt,
		})
		if err != nil {
			return FormationExecutionResult{}, err
		}
		if err := dispatcher.CompleteFromCapture(req.RunID, lease.DispatchID, fmt.Sprintf("<<<CHROTE-DONE run-id=%s status=ok artifact=lab-%s.md>>>", req.RunID, slot.ID)); err != nil {
			return FormationExecutionResult{}, err
		}
		outputs = append(outputs, e.renderSlotOutput(req, slot, *card, variant))
	}

	text := strings.Join(outputs, "\n\n")
	if len(text) > e.config.OutputCapBytes {
		text = text[:e.config.OutputCapBytes]
	}
	return FormationExecutionResult{
		Status:    "done",
		ReportRef: fmt.Sprintf("lab://%s/%s/report.md", req.RunID, req.NodeID),
		Text:      text,
		Outputs:   labOutputPayloads(req.Formation, text),
	}, nil
}

func (e *LabFormationExecutor) validateConfiguredBoundary() error {
	if strings.TrimSpace(e.config.Cwd) == "" {
		return runExecutionError("missing_cwd", "lab executor cwd is not configured", "executor", nil)
	}
	if len(e.config.Roots) == 0 {
		return runExecutionError("missing_root", "lab executor root is not configured", "executor", nil)
	}
	cwd, err := filepath.Abs(e.config.Cwd)
	if err != nil {
		return runExecutionError("invalid_cwd", "lab executor cwd is invalid", "executor", err)
	}
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return runExecutionError("unavailable_cwd", "lab executor cwd is unavailable", "executor", err)
	}
	for _, root := range e.config.Roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return runExecutionError("invalid_root", "lab executor root is invalid", "executor", err)
		}
		if info, err := os.Stat(absRoot); err != nil || !info.IsDir() {
			return runExecutionError("unavailable_root", "lab executor root is unavailable", "executor", err)
		}
		if pathWithinRoot(cwd, absRoot) {
			return nil
		}
	}
	return runExecutionError("cwd_outside_root", "lab executor cwd is outside configured roots", "executor", nil)
}

func (e *LabFormationExecutor) allowedHarnesses() map[string]bool {
	allowed := make(map[string]bool, len(e.config.Harnesses))
	for _, harness := range e.config.Harnesses {
		if harness != "" {
			allowed[harness] = true
		}
	}
	return allowed
}

func (e *LabFormationExecutor) renderPrompt(req FormationExecution, slot FormationSlot, card PersonaCard, variant HarnessVariant) string {
	var b strings.Builder
	b.WriteString("run: " + req.RunID + "\n")
	b.WriteString("node: " + req.NodeID + "\n")
	b.WriteString("slot: " + slot.ID + "\n")
	b.WriteString("agent: " + card.ID + "\n")
	b.WriteString("harness: " + variant.ID + "\n")
	b.WriteString("cwd: " + e.config.Cwd + "\n")
	b.WriteString("brief: " + req.Brief.Goal + "\n")
	for _, input := range req.Inputs {
		if input.Text != "" {
			b.WriteString("input: " + input.Text + "\n")
		}
	}
	return b.String()
}

func (e *LabFormationExecutor) renderSlotOutput(req FormationExecution, slot FormationSlot, card PersonaCard, variant HarnessVariant) string {
	inputs := make([]string, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		if input.Text != "" {
			inputs = append(inputs, input.Text)
		}
	}
	inputText := strings.Join(inputs, "\n")
	if inputText == "" {
		inputText = req.Brief.Goal
	}
	return fmt.Sprintf("lab-fake output from %s using %s for %s\nslot: %s\ninput: %s", card.ID, variant.ID, req.Title, slot.ID, inputText)
}

func labOutputPayloads(formation FormationNode, text string) map[string]FormationOutputPayload {
	outputs := make(map[string]FormationOutputPayload, len(formation.Outputs))
	multiOutput := len(formation.Outputs) > 1
	for _, port := range formation.Outputs {
		payloadText := text
		if multiOutput {
			payloadText = fmt.Sprintf("%s\noutput-port: %s", text, port.ID)
		}
		outputs[port.ID] = FormationOutputPayload{
			Text: payloadText,
		}
	}
	return outputs
}

func runExecutionError(code, message, boundary string, cause error) error {
	return &RunExecutionError{
		Code:     code,
		Message:  redactLedgerText(message),
		Boundary: boundary,
		Cause:    cause,
	}
}

func splitLabCSV(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func pathWithinRoot(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}
