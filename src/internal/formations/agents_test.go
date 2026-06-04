package formations

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreatePersonaWritesOneIDSpineAndDefaultSessionStem(t *testing.T) {
	store := NewPersonaStore(t.TempDir())
	store.Now = fixedClock()

	card, err := store.CreatePersona(CreatePersonaRequest{
		ID:           "scout",
		Kind:         "specialist",
		Harness:      "claude-code",
		Capabilities: []string{"research", "go"},
		Personality:  "direct",
		Source:       "/tmp/CLAUDE.md",
	})
	if err != nil {
		t.Fatalf("create persona: %v", err)
	}

	if card.ID != "scout" {
		t.Fatalf("card.ID = %q, want scout", card.ID)
	}
	if got := card.DefaultVariant().SessionStem; got != "scout" {
		t.Fatalf("default session stem = %q, want card id", got)
	}
	if got := filepath.Base(store.PersonaPath("scout")); got != "scout.toml" {
		t.Fatalf("persona path base = %q, want scout.toml", got)
	}
	if !containsAll(card.Tags, []string{"research", "go", "personality:direct"}) {
		t.Fatalf("tags = %#v, want capabilities and personality facet", card.Tags)
	}

	raw := readFile(t, store.PersonaPath("scout"))
	for _, want := range []string{
		"schema = 1",
		`id = "scout"`,
		`kind = "specialist"`,
		`default = "claude-code"`,
		`session_stem = "scout"`,
		`source = "/tmp/CLAUDE.md"`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("persona TOML missing %q:\n%s", want, raw)
		}
	}
}

func TestCreatePersonaRefusesExistingIDWithoutClobber(t *testing.T) {
	store := NewPersonaStore(t.TempDir())
	existing := `schema = 1

[card]
id = "scout"
kind = "specialist"
tags = ["research"]
reviewerNotes = "keep this exact line"

[harness]
default = "claude-code"

[[harness.variant]]
id = "claude-code"
session_stem = "scout"
`
	writeFixture(t, store.PersonaPath("scout"), existing)

	_, err := store.CreatePersona(CreatePersonaRequest{ID: "scout", Kind: "specialist", Harness: "openai-codex"})
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("create existing error = %v, want ErrAlreadyExists", err)
	}
	if got := readFile(t, store.PersonaPath("scout")); got != existing {
		t.Fatalf("existing card changed:\n--- got ---\n%s\n--- want ---\n%s", got, existing)
	}
}

func TestEditPersonaCapabilitiesPreservesUnknownFieldsAndIsIdempotent(t *testing.T) {
	store := NewPersonaStore(t.TempDir())
	writeFixture(t, store.PersonaPath("susie"), `schema = 1

[card]
id = "susie"
display_name = "Susie"
kind = "specialist"
tags = ["design", "react", "taste:visual"]
reviewerNotes = "prefers tight grids"

[harness]
default = "claude-code"

[[harness.variant]]
id = "claude-code"
session_stem = "susie"
source = "/tmp/CLAUDE.md"
`)

	editPersonaWithFreshETag(t, store, "susie", EditPersonaRequest{AddCapability: "tailwind"})
	editPersonaWithFreshETag(t, store, "susie", EditPersonaRequest{RemoveCapability: "tailwind"})
	card := editPersonaWithFreshETag(t, store, "susie", EditPersonaRequest{RemoveCapability: "tailwind"})

	if contains(card.Tags, "tailwind") {
		t.Fatalf("tags still contain removed capability: %#v", card.Tags)
	}
	raw := readFile(t, store.PersonaPath("susie"))
	if !strings.Contains(raw, `reviewerNotes = "prefers tight grids"`) {
		t.Fatalf("unknown field was not preserved:\n%s", raw)
	}
}

func TestEditPersonaAddsHarnessVariantAndNote(t *testing.T) {
	store := NewPersonaStore(t.TempDir())
	store.Now = func() time.Time { return time.Date(2026, 6, 3, 18, 0, 0, 0, time.UTC) }
	writeFixture(t, store.PersonaPath("susie"), minimalPersona("susie", "specialist", []string{"design"}))

	card := editPersonaWithFreshETag(t, store, "susie", EditPersonaRequest{
		AddHarness:  "hermes",
		SessionStem: "hermes-susie",
		Note:        "react quality improved over sprint 3",
	})
	if len(card.HarnessVariants) != 2 {
		t.Fatalf("harness variants = %d, want 2", len(card.HarnessVariants))
	}
	if got := card.HarnessVariants[1].SessionStem; got != "hermes-susie" {
		t.Fatalf("new harness session stem = %q, want hermes-susie", got)
	}
	if len(card.Notes) != 1 || card.Notes[0].Text != "react quality improved over sprint 3" {
		t.Fatalf("notes = %#v", card.Notes)
	}
}

func TestEditPersonaRequiresPreconditionBeforeWriting(t *testing.T) {
	store := NewPersonaStore(t.TempDir())
	writeFixture(t, store.PersonaPath("susie"), minimalPersona("susie", "specialist", []string{"design"}))
	before := readFile(t, store.PersonaPath("susie"))

	_, err := store.EditPersona("susie", EditPersonaRequest{AddCapability: "react"})
	if !errors.Is(err, ErrPreconditionRequired) {
		t.Fatalf("edit without expected ETag error = %v, want ErrPreconditionRequired", err)
	}
	if got := readFile(t, store.PersonaPath("susie")); got != before {
		t.Fatalf("card changed despite missing precondition:\n%s", got)
	}

	fresh, err := store.ReadPersona("susie")
	if err != nil {
		t.Fatalf("read persona: %v", err)
	}
	updated, err := store.EditPersona("susie", EditPersonaRequest{AddCapability: "react", ExpectedETag: fresh.ETag})
	if err != nil {
		t.Fatalf("edit with matching ETag: %v", err)
	}
	if !contains(updated.Tags, "react") || updated.ETag == fresh.ETag {
		t.Fatalf("updated card = %+v, want react and changed ETag", updated)
	}
	afterMatch := readFile(t, store.PersonaPath("susie"))

	_, err = store.EditPersona("susie", EditPersonaRequest{AddCapability: "go", ExpectedETag: fresh.ETag})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("edit with stale ETag error = %v, want ErrConflict", err)
	}
	if got := readFile(t, store.PersonaPath("susie")); got != afterMatch {
		t.Fatalf("stale edit changed card:\n%s", got)
	}
}

func TestAgentRosterLeftJoinsCardsWithLiveSessions(t *testing.T) {
	cards := []PersonaCard{
		mustParsePersonaFixture(t, "susie", minimalPersona("susie", "specialist", []string{"design", "react"})),
		mustParsePersonaFixture(t, "codex", minimalPersona("codex", "specialist", []string{"go", "fast"})),
	}
	live := []LiveAgentSession{
		{Name: "susie", Status: "idle", Attached: true},
		{Name: "scratch", Status: "working"},
	}

	roster, err := ProjectAgentRoster(cards, live, AgentRosterFilter{})
	if err != nil {
		t.Fatalf("project roster: %v", err)
	}
	if got := roster.ByID("susie").Liveness; got != AgentLivenessLive {
		t.Fatalf("susie liveness = %q, want live", got)
	}
	if got := roster.ByID("codex").Liveness; got != AgentLivenessOffline {
		t.Fatalf("codex liveness = %q, want offline", got)
	}
	scratch := roster.ByID("scratch")
	if scratch == nil || !scratch.Unbound || scratch.Assignable {
		t.Fatalf("scratch projection = %#v, want visible unbound and not assignable", scratch)
	}

	assignable, err := ProjectAgentRoster(cards, live, AgentRosterFilter{AssignableOnly: true})
	if err != nil {
		t.Fatalf("project assignable roster: %v", err)
	}
	if assignable.ByID("scratch") != nil {
		t.Fatalf("unbound scratch should be excluded from assignable roster: %#v", assignable)
	}
}

func TestAgentRosterFiltersBareCapabilitiesOnly(t *testing.T) {
	cards := []PersonaCard{
		mustParsePersonaFixture(t, "susie", minimalPersona("susie", "specialist", []string{"react", "taste:visual"})),
		mustParsePersonaFixture(t, "codex", minimalPersona("codex", "specialist", []string{"typescript"})),
	}

	roster, err := ProjectAgentRoster(cards, nil, AgentRosterFilter{Capable: "react"})
	if err != nil {
		t.Fatalf("project roster: %v", err)
	}
	if roster.ByID("susie") == nil {
		t.Fatal("susie missing from react-capable roster")
	}
	if roster.ByID("codex") != nil {
		t.Fatal("codex included in react-capable roster")
	}

	roster, err = ProjectAgentRoster(cards, nil, AgentRosterFilter{Capable: "taste:visual"})
	if err != nil {
		t.Fatalf("project roster by facet: %v", err)
	}
	if len(roster.Agents) != 0 {
		t.Fatalf("facet tag matched as capability: %#v", roster.Agents)
	}
}

func TestResolveAgentSessionUsesDeclaredHarnessStem(t *testing.T) {
	card := mustParsePersonaFixture(t, "susie", multiHarnessPersona("susie"))
	live := []LiveAgentSession{
		{Name: "claude-susie", Status: "idle"},
		{Name: "codex-susie", Status: "working"},
	}

	binding, err := ResolveAgentSession(card, live, "claude-code")
	if err != nil {
		t.Fatalf("resolve claude-code: %v", err)
	}
	if binding.Harness != "claude-code" || binding.SessionStem != "claude-susie" || binding.Session.Name != "claude-susie" {
		t.Fatalf("claude binding = %#v", binding)
	}

	binding, err = ResolveAgentSession(card, live, "openai-codex")
	if err != nil {
		t.Fatalf("resolve openai-codex: %v", err)
	}
	if binding.Harness != "openai-codex" || binding.SessionStem != "codex-susie" || binding.Session.Name != "codex-susie" {
		t.Fatalf("codex binding = %#v", binding)
	}
}

func TestResolveAgentSessionFailsLoudWhenHarnessIsAmbiguous(t *testing.T) {
	card := mustParsePersonaFixture(t, "susie", multiHarnessPersona("susie"))

	_, err := ResolveAgentSession(card, []LiveAgentSession{
		{Name: "claude-susie"},
		{Name: "codex-susie"},
	}, "")
	if !errors.Is(err, ErrAmbiguousAgentBinding) {
		t.Fatalf("resolve without harness error = %v, want ErrAmbiguousAgentBinding", err)
	}
}

func TestResolveAgentSessionFailsLoudForOfflineOrDuplicateLiveMatches(t *testing.T) {
	card := mustParsePersonaFixture(t, "scout", minimalPersona("scout", "specialist", []string{"research"}))

	_, err := ResolveAgentSession(card, nil, "")
	if !errors.Is(err, ErrAgentSessionOffline) {
		t.Fatalf("offline resolve error = %v, want ErrAgentSessionOffline", err)
	}

	_, err = ResolveAgentSession(card, []LiveAgentSession{{Name: "scout"}, {Name: "scout"}}, "")
	if !errors.Is(err, ErrAmbiguousAgentBinding) {
		t.Fatalf("duplicate live resolve error = %v, want ErrAmbiguousAgentBinding", err)
	}
}

func mustParsePersonaFixture(t *testing.T, id, raw string) PersonaCard {
	t.Helper()
	card, err := parsePersonaCard(id, []byte(raw))
	if err != nil {
		t.Fatalf("parse persona fixture %s: %v", id, err)
	}
	return *card
}

func editPersonaWithFreshETag(t *testing.T, store *PersonaStore, id string, req EditPersonaRequest) *PersonaCard {
	t.Helper()
	before, err := store.ReadPersona(id)
	if err != nil {
		t.Fatalf("read persona %s: %v", id, err)
	}
	req.ExpectedETag = before.ETag
	card, err := store.EditPersona(id, req)
	if err != nil {
		t.Fatalf("edit persona %s: %v", id, err)
	}
	return card
}

func minimalPersona(id, kind string, tags []string) string {
	return `schema = 1

[card]
id = "` + id + `"
display_name = "` + strings.Title(id) + `"
kind = "` + kind + `"
tags = [` + renderStringList(tags) + `]

[harness]
default = "claude-code"

[[harness.variant]]
id = "claude-code"
session_stem = "` + id + `"
`
}

func multiHarnessPersona(id string) string {
	return `schema = 1

[card]
id = "` + id + `"
display_name = "` + strings.Title(id) + `"
kind = "specialist"
tags = ["design", "react"]

[harness]
default = "claude-code"

[[harness.variant]]
id = "claude-code"
session_stem = "claude-` + id + `"

[[harness.variant]]
id = "openai-codex"
session_stem = "codex-` + id + `"
`
}

func containsAll(values, wants []string) bool {
	for _, want := range wants {
		if !contains(values, want) {
			return false
		}
	}
	return true
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestPersonaPathValidationRejectsNonSlugIDs(t *testing.T) {
	store := NewPersonaStore(t.TempDir())
	for _, id := range []string{"", "../scout", "Scout", "scout.toml", "scout/slash", "bad id", "agent:one", "_hidden", "-bad", "bad-"} {
		if _, err := store.CreatePersona(CreatePersonaRequest{ID: id, Kind: "specialist", Harness: "claude-code"}); !errors.Is(err, ErrInvalidSlug) {
			t.Fatalf("CreatePersona(%q) error = %v, want ErrInvalidSlug", id, err)
		}
	}
	entries, err := os.ReadDir(store.AgentsDir)
	if err != nil {
		t.Fatalf("read temp agents dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid create unexpectedly wrote files: %#v", entries)
	}
}
