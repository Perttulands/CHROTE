package formations

import (
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestCodexPersonaPresetsAreAvailableWithoutPersistedCards(t *testing.T) {
	store := NewPersonaStore(t.TempDir())
	cards, err := store.ListPersonas()
	if err != nil {
		t.Fatalf("list presets: %v", err)
	}
	if len(cards) != 7 {
		t.Fatalf("preset count = %d, want 7", len(cards))
	}
	gotIDs := make([]string, 0, len(cards))
	for _, card := range cards {
		gotIDs = append(gotIDs, card.ID)
		if !card.Preset || card.Customized || card.HarnessDefault != "openai-codex" {
			t.Fatalf("preset projection = %+v", card)
		}
		variant := card.DefaultVariant()
		if variant.ID != "openai-codex" || variant.SessionStem != card.ID || variant.Launch != "codex --yolo -c check_for_update_on_startup=false" {
			t.Fatalf("preset harness = %+v", variant)
		}
	}
	wantIDs := []string{"codex-builder", "codex-debugger", "codex-judge", "codex-orchestrator", "codex-planner", "codex-reviewer", "codex-scout"}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("preset ids = %v, want %v", gotIDs, wantIDs)
	}
}

func TestEditingCodexPresetMaterializesLocalOverride(t *testing.T) {
	dir := t.TempDir()
	store := NewPersonaStore(dir)
	builtin, err := store.ReadPersona("codex-builder")
	if err != nil {
		t.Fatalf("read builtin: %v", err)
	}
	name := "Repository Builder"
	summary := "Builds in the selected workspace"
	capabilities := []string{"implement", "test", "refactor"}
	stem := "codex-builder-main"
	launch := "codex --yolo --model gpt-5.6-codex"
	updated, err := store.EditPersona("codex-builder", EditPersonaRequest{
		SetDisplayName:  &name,
		SetSummary:      &summary,
		SetCapabilities: &capabilities,
		SetSessionStem:  &stem,
		SetLaunch:       &launch,
		ExpectedETag:    builtin.ETag,
	})
	if err != nil {
		t.Fatalf("edit builtin: %v", err)
	}
	if !updated.Preset || !updated.Customized || updated.DisplayName != name || updated.Summary != summary {
		t.Fatalf("updated preset = %+v", updated)
	}
	if !reflect.DeepEqual(bareCapabilities(updated.Tags), capabilities) {
		t.Fatalf("capabilities = %v, want %v", updated.Tags, capabilities)
	}
	variant := updated.DefaultVariant()
	if variant.SessionStem != stem || variant.Launch != launch {
		t.Fatalf("updated harness = %+v", variant)
	}
	if _, err := os.Stat(store.PersonaPath("codex-builder")); err != nil {
		t.Fatalf("materialized override: %v", err)
	}
	reread, err := NewPersonaStore(dir).ReadPersona("codex-builder")
	if err != nil || !reread.Customized || reread.ETag != updated.ETag {
		t.Fatalf("reread override = %+v, err %v", reread, err)
	}
}

func TestStaleCodexPresetEditDoesNotMaterializeOverride(t *testing.T) {
	store := NewPersonaStore(t.TempDir())
	name := "Stale"
	_, err := store.EditPersona("codex-scout", EditPersonaRequest{SetDisplayName: &name, ExpectedETag: "stale"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("stale edit error = %v, want ErrConflict", err)
	}
	if _, err := os.Stat(store.PersonaPath("codex-scout")); !os.IsNotExist(err) {
		t.Fatalf("stale edit created override: %v", err)
	}
}

func bareCapabilities(tags []string) []string {
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		if isBareCapability(tag) {
			result = append(result, tag)
		}
	}
	return result
}
