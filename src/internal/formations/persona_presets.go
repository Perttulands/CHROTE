package formations

import "sort"

type codexPersonaPreset struct {
	ID           string
	DisplayName  string
	Kind         string
	Summary      string
	Capabilities []string
}

var codexPersonaPresetCatalog = []codexPersonaPreset{
	{ID: "codex-scout", DisplayName: "Codex Scout", Kind: "scout", Summary: "Explores a codebase, gathers evidence, and reports the terrain before changes begin.", Capabilities: []string{"research", "inspect"}},
	{ID: "codex-planner", DisplayName: "Codex Planner", Kind: "planner", Summary: "Turns grounded requirements into an ordered implementation plan with explicit verification.", Capabilities: []string{"planning", "design"}},
	{ID: "codex-builder", DisplayName: "Codex Builder", Kind: "builder", Summary: "Implements scoped changes, exercises them, and leaves a working artifact.", Capabilities: []string{"implement", "test"}},
	{ID: "codex-judge", DisplayName: "Codex Judge", Kind: "judge", Summary: "Evaluates evidence against acceptance criteria and returns a clear verdict.", Capabilities: []string{"judge", "verify"}},
	{ID: "codex-orchestrator", DisplayName: "Codex Orchestrator", Kind: "orchestrator", Summary: "Coordinates role-specialized agents, dependencies, handoffs, and completion gates.", Capabilities: []string{"orchestrate", "coordinate"}},
	{ID: "codex-debugger", DisplayName: "Codex Debugger", Kind: "debugger", Summary: "Diagnoses failures from evidence, isolates root causes, and verifies repairs.", Capabilities: []string{"debug", "diagnose"}},
	{ID: "codex-reviewer", DisplayName: "Codex Reviewer", Kind: "reviewer", Summary: "Reviews changes independently for correctness, regressions, and maintainability.", Capabilities: []string{"review", "audit"}},
}

func codexPresetPersona(id string) (*PersonaCard, bool) {
	for _, preset := range codexPersonaPresetCatalog {
		if preset.ID != id {
			continue
		}
		tags := append([]string{}, preset.Capabilities...)
		tags = append(tags, "role:"+preset.Kind, "provider:openai-codex")
		req := CreatePersonaRequest{
			ID:           preset.ID,
			DisplayName:  preset.DisplayName,
			Kind:         preset.Kind,
			Summary:      preset.Summary,
			Capabilities: tags,
			Harness:      "openai-codex",
			SessionStem:  preset.ID,
			Launch:       "codex --yolo -c check_for_update_on_startup=false",
		}
		raw := renderPersona(req, req.Harness, req.SessionStem, normalizeTags(req.Capabilities))
		card, err := parsePersonaCard(id, []byte(raw))
		if err != nil {
			panic("invalid built-in Codex persona preset " + id + ": " + err.Error())
		}
		card.Preset = true
		return card, true
	}
	return nil, false
}

func codexPresetPersonas() []PersonaCard {
	cards := make([]PersonaCard, 0, len(codexPersonaPresetCatalog))
	for _, preset := range codexPersonaPresetCatalog {
		card, _ := codexPresetPersona(preset.ID)
		cards = append(cards, *card)
	}
	sort.Slice(cards, func(i, j int) bool { return cards[i].ID < cards[j].ID })
	return cards
}
