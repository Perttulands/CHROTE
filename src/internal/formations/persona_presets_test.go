package formations

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
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

func TestPersonaStoreRejectsSymlinkAndFIFOCardSubstitution(t *testing.T) {
	dir := t.TempDir()
	external := filepath.Join(t.TempDir(), "external.toml")
	externalRaw := renderPersona(CreatePersonaRequest{
		ID:          "codex-builder",
		DisplayName: "Substituted Builder",
		Kind:        "builder",
		Summary:     "external secret",
	}, "openai-codex", "codex-builder", []string{"implement"})
	if err := os.WriteFile(external, []byte(externalRaw), 0o600); err != nil {
		t.Fatalf("write external file: %v", err)
	}
	if err := os.Symlink(external, filepath.Join(dir, "codex-builder.toml")); err != nil {
		t.Fatalf("symlink card: %v", err)
	}
	store := NewPersonaStore(dir)
	if _, err := store.ReadPersona("codex-builder"); err == nil {
		t.Fatal("ReadPersona followed substituted symlink")
	}
	if got, err := os.ReadFile(external); err != nil || string(got) != externalRaw {
		t.Fatalf("external file changed: %q, err=%v", got, err)
	}

	if err := os.Remove(filepath.Join(dir, "codex-builder.toml")); err != nil {
		t.Fatalf("remove card symlink: %v", err)
	}
	fifo := filepath.Join(dir, "codex-builder.toml")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}
	if _, err := store.ReadPersona("codex-builder"); err == nil {
		t.Fatal("ReadPersona accepted FIFO card")
	}
}

func TestPersonaStoreRejectsSymlinkLockWithoutChangingExternalMode(t *testing.T) {
	dir := t.TempDir()
	external := filepath.Join(t.TempDir(), "external-lock")
	if err := os.WriteFile(external, []byte("lock sentinel"), 0o600); err != nil {
		t.Fatalf("write external lock: %v", err)
	}
	if err := os.Symlink(external, filepath.Join(dir, "codex-scout.toml.lock")); err != nil {
		t.Fatalf("symlink lock: %v", err)
	}
	store := NewPersonaStore(dir)
	builtin, err := store.ReadPersona("codex-scout")
	if err != nil {
		t.Fatalf("read builtin: %v", err)
	}
	name := "Unsafe override"
	if _, err := store.EditPersona("codex-scout", EditPersonaRequest{SetDisplayName: &name, ExpectedETag: builtin.ETag}); err == nil {
		t.Fatal("EditPersona followed substituted lock symlink")
	}
	info, err := os.Stat(external)
	if err != nil {
		t.Fatalf("stat external lock: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("external lock mode = %o, want 600", info.Mode().Perm())
	}
	if _, err := os.Stat(store.PersonaPath("codex-scout")); !os.IsNotExist(err) {
		t.Fatalf("unsafe edit materialized card: %v", err)
	}

	lockPath := filepath.Join(dir, "codex-scout.toml.lock")
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("remove lock symlink: %v", err)
	}
	if err := syscall.Mkfifo(lockPath, 0o600); err != nil {
		t.Fatalf("mkfifo lock: %v", err)
	}
	if _, err := store.EditPersona("codex-scout", EditPersonaRequest{SetDisplayName: &name, ExpectedETag: builtin.ETag}); err == nil {
		t.Fatal("EditPersona accepted FIFO lock")
	}
	if _, err := os.Stat(store.PersonaPath("codex-scout")); !os.IsNotExist(err) {
		t.Fatalf("FIFO lock edit materialized card: %v", err)
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
