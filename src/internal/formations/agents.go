package formations

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrAgentSessionOffline   = errors.New("agent session offline")
	ErrAlreadyExists         = errors.New("formations file already exists")
	ErrAmbiguousAgentBinding = errors.New("ambiguous agent binding")
)

const (
	AgentLivenessLive      = "live"
	AgentLivenessOffline   = "offline"
	AgentLivenessAmbiguous = "ambiguous"
)

type PersonaStore struct {
	AgentsDir string
	Now       func() time.Time
}

type PersonaCard struct {
	Schema          int              `json:"schema"`
	ID              string           `json:"id"`
	DisplayName     string           `json:"displayName,omitempty"`
	Kind            string           `json:"kind"`
	Summary         string           `json:"summary,omitempty"`
	Tags            []string         `json:"tags"`
	Status          string           `json:"status,omitempty"`
	HarnessDefault  string           `json:"harnessDefault"`
	HarnessVariants []HarnessVariant `json:"harnessVariants"`
	Notes           []PersonaNote    `json:"notes,omitempty"`
	ETag            string           `json:"etag"`
	TOML            string           `json:"toml,omitempty"`
}

type HarnessVariant struct {
	ID          string `json:"id"`
	SessionStem string `json:"sessionStem,omitempty"`
	Launch      string `json:"launch,omitempty"`
	Source      string `json:"source,omitempty"`
}

type PersonaNote struct {
	Timestamp string `json:"ts"`
	Actor     string `json:"actor"`
	Text      string `json:"text"`
}

type CreatePersonaRequest struct {
	ID           string
	DisplayName  string
	Kind         string
	Summary      string
	Capabilities []string
	Personality  string
	Harness      string
	SessionStem  string
	Launch       string
	Source       string
}

type EditPersonaRequest struct {
	AddCapability    string
	RemoveCapability string
	AddHarness       string
	SessionStem      string
	Launch           string
	Source           string
	Note             string
	Retire           bool
	ExpectedETag     string
}

type AgentRosterFilter struct {
	Capable        string
	AssignableOnly bool
}

type LiveAgentSession struct {
	Name       string `json:"name"`
	Status     string `json:"status,omitempty"`
	ContextPct int    `json:"contextPct,omitempty"`
	BeadID     string `json:"beadId,omitempty"`
	Attached   bool   `json:"attached"`
}

type AgentRoster struct {
	Agents []AgentProjection `json:"agents"`
}

type AgentProjection struct {
	ID             string   `json:"id"`
	DisplayName    string   `json:"displayName,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	HarnessDefault string   `json:"harnessDefault,omitempty"`
	Liveness       string   `json:"liveness"`
	SessionID      string   `json:"sessionId,omitempty"`
	Status         string   `json:"status,omitempty"`
	ContextPct     int      `json:"contextPct,omitempty"`
	BeadID         string   `json:"beadId,omitempty"`
	Attached       bool     `json:"attached"`
	Assignable     bool     `json:"assignable"`
	Unbound        bool     `json:"unbound,omitempty"`
}

type AgentSessionBinding struct {
	AgentID     string           `json:"agentId"`
	Harness     string           `json:"harness"`
	SessionStem string           `json:"sessionStem"`
	Session     LiveAgentSession `json:"session"`
}

func NewPersonaStore(agentsDir string) *PersonaStore {
	return &PersonaStore{
		AgentsDir: agentsDir,
		Now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func DefaultAgentsDir() string {
	if dir := strings.TrimSpace(os.Getenv("CHROTE_AGENTS_DIR")); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "agents"
	}
	return filepath.Join(home, "agents")
}

func (s *PersonaStore) PersonaPath(id string) string {
	return filepath.Join(s.AgentsDir, id+".toml")
}

func (s *PersonaStore) ListPersonas() ([]PersonaCard, error) {
	entries, err := os.ReadDir(s.AgentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []PersonaCard{}, nil
		}
		return nil, err
	}
	cards := make([]PersonaCard, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".toml")
		card, err := s.ReadPersona(id)
		if err != nil {
			return nil, err
		}
		cards = append(cards, *card)
	}
	sort.Slice(cards, func(i, j int) bool {
		return cards[i].ID < cards[j].ID
	})
	return cards, nil
}

func (s *PersonaStore) ReadPersona(id string) (*PersonaCard, error) {
	if err := validatePersonaID(id); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(s.PersonaPath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return parsePersonaCard(id, raw)
}

func (s *PersonaStore) CreatePersona(req CreatePersonaRequest) (*PersonaCard, error) {
	if err := validatePersonaID(req.ID); err != nil {
		return nil, err
	}
	path := s.PersonaPath(req.ID)
	var created *PersonaCard
	err := withFileLock(path, func() error {
		if _, err := os.Stat(path); err == nil {
			return ErrAlreadyExists
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		if req.Kind == "" {
			return fmt.Errorf("%w: kind is required", ErrInvalidSlug)
		}
		harness := req.Harness
		if harness == "" {
			harness = inferHarness(req.Source)
		}
		if harness == "" {
			harness = "claude-code"
		}
		sessionStem := req.SessionStem
		if sessionStem == "" {
			sessionStem = req.ID
		}
		launch := req.Launch
		if launch == "" {
			launch = inferLaunch(harness, req.Source)
		}
		tags := normalizeTags(req.Capabilities)
		if req.Personality != "" {
			tags = appendUnique(tags, "personality:"+req.Personality)
		}
		req.Launch = launch
		raw := renderPersona(req, harness, sessionStem, tags)
		if err := writeAtomic(path, []byte(raw)); err != nil {
			return err
		}
		card, err := parsePersonaCard(req.ID, []byte(raw))
		if err != nil {
			return err
		}
		created = card
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *PersonaStore) EditPersona(id string, req EditPersonaRequest) (*PersonaCard, error) {
	if err := validatePersonaID(id); err != nil {
		return nil, err
	}
	path := s.PersonaPath(id)
	var updated *PersonaCard
	err := withFileLock(path, func() error {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return ErrNotFound
			}
			return err
		}
		card, err := parsePersonaCard(id, raw)
		if err != nil {
			return err
		}
		if req.ExpectedETag == "" {
			return ErrPreconditionRequired
		}
		if req.ExpectedETag != card.ETag {
			return ErrConflict
		}
		next := string(raw)
		if req.AddCapability != "" || req.RemoveCapability != "" {
			tags := append([]string{}, card.Tags...)
			if req.AddCapability != "" && isBareCapability(req.AddCapability) {
				tags = appendUnique(tags, req.AddCapability)
			}
			if req.RemoveCapability != "" && isBareCapability(req.RemoveCapability) {
				tags = removeValue(tags, req.RemoveCapability)
			}
			next = setSectionScalar(next, "card", "tags", "["+renderStringList(tags)+"]")
		}
		if req.Retire {
			next = setSectionScalar(next, "card", "status", renderString("retired"))
		}
		if req.AddHarness != "" {
			stem := req.SessionStem
			if stem == "" {
				stem = req.AddHarness + "-" + id
			}
			launch := req.Launch
			if launch == "" {
				launch = inferLaunch(req.AddHarness, req.Source)
			}
			next = appendHarnessVariant(next, HarnessVariant{
				ID:          req.AddHarness,
				SessionStem: stem,
				Launch:      launch,
				Source:      req.Source,
			})
		}
		if req.Note != "" {
			next = appendPersonaNote(next, PersonaNote{
				Timestamp: s.now().Format(time.RFC3339),
				Actor:     "agent:archon",
				Text:      req.Note,
			})
		}
		if err := writeAtomic(path, []byte(next)); err != nil {
			return err
		}
		updated, err = parsePersonaCard(id, []byte(next))
		return err
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *PersonaStore) now() time.Time {
	if s.Now == nil {
		return time.Now().UTC()
	}
	return s.Now().UTC()
}

func (c PersonaCard) DefaultVariant() HarnessVariant {
	for _, variant := range c.HarnessVariants {
		if variant.ID == c.HarnessDefault {
			if variant.SessionStem == "" {
				variant.SessionStem = c.ID
			}
			return variant
		}
	}
	if len(c.HarnessVariants) == 0 {
		return HarnessVariant{ID: c.HarnessDefault, SessionStem: c.ID}
	}
	variant := c.HarnessVariants[0]
	if variant.SessionStem == "" {
		variant.SessionStem = c.ID
	}
	return variant
}

func (c PersonaCard) SelectHarnessVariant(harness string) (HarnessVariant, error) {
	if harness == "" {
		if len(c.HarnessVariants) > 1 {
			return HarnessVariant{}, fmt.Errorf("%w: agent %q has multiple harness variants; choose a harness", ErrAmbiguousAgentBinding, c.ID)
		}
		return c.DefaultVariant(), nil
	}
	for _, variant := range c.HarnessVariants {
		if variant.ID != harness {
			continue
		}
		if variant.SessionStem == "" && variant.ID == c.HarnessDefault {
			variant.SessionStem = c.ID
		}
		return variant, nil
	}
	return HarnessVariant{}, fmt.Errorf("%w: agent %q has no harness variant %q", ErrNotFound, c.ID, harness)
}

func ResolveAgentSession(card PersonaCard, live []LiveAgentSession, harness string) (AgentSessionBinding, error) {
	variant, err := card.SelectHarnessVariant(harness)
	if err != nil {
		return AgentSessionBinding{}, err
	}
	if variant.SessionStem == "" {
		return AgentSessionBinding{}, fmt.Errorf("%w: agent %q harness %q has no session_stem", ErrAgentSessionOffline, card.ID, variant.ID)
	}

	matches := make([]LiveAgentSession, 0, 1)
	for _, session := range live {
		if session.Name == variant.SessionStem {
			matches = append(matches, session)
		}
	}
	switch len(matches) {
	case 0:
		return AgentSessionBinding{}, fmt.Errorf("%w: no live session for agent %q harness %q stem %q", ErrAgentSessionOffline, card.ID, variant.ID, variant.SessionStem)
	case 1:
		return AgentSessionBinding{
			AgentID:     card.ID,
			Harness:     variant.ID,
			SessionStem: variant.SessionStem,
			Session:     matches[0],
		}, nil
	default:
		return AgentSessionBinding{}, fmt.Errorf("%w: agent %q harness %q matched %d live sessions for stem %q", ErrAmbiguousAgentBinding, card.ID, variant.ID, len(matches), variant.SessionStem)
	}
}

func ProjectAgentRoster(cards []PersonaCard, live []LiveAgentSession, filter AgentRosterFilter) (AgentRoster, error) {
	usedSessions := map[string]bool{}
	projections := make([]AgentProjection, 0, len(cards)+len(live))
	for _, card := range cards {
		if filter.Capable != "" && !hasBareCapability(card.Tags, filter.Capable) {
			continue
		}
		projection := projectCard(card, live)
		if projection.SessionID != "" {
			usedSessions[projection.SessionID] = true
		}
		if filter.AssignableOnly && !projection.Assignable {
			continue
		}
		projections = append(projections, projection)
	}
	if filter.Capable == "" {
		for _, session := range live {
			if usedSessions[session.Name] {
				continue
			}
			projection := AgentProjection{
				ID:          session.Name,
				DisplayName: session.Name,
				Liveness:    AgentLivenessLive,
				SessionID:   session.Name,
				Status:      session.Status,
				ContextPct:  session.ContextPct,
				BeadID:      session.BeadID,
				Attached:    session.Attached,
				Assignable:  false,
				Unbound:     true,
			}
			if !filter.AssignableOnly {
				projections = append(projections, projection)
			}
		}
	}
	sort.Slice(projections, func(i, j int) bool {
		if projections[i].Unbound != projections[j].Unbound {
			return !projections[i].Unbound
		}
		return projections[i].ID < projections[j].ID
	})
	return AgentRoster{Agents: projections}, nil
}

func (r AgentRoster) ByID(id string) *AgentProjection {
	for i := range r.Agents {
		if r.Agents[i].ID == id {
			return &r.Agents[i]
		}
	}
	return nil
}

func projectCard(card PersonaCard, live []LiveAgentSession) AgentProjection {
	projection := AgentProjection{
		ID:             card.ID,
		DisplayName:    card.DisplayName,
		Kind:           card.Kind,
		Tags:           append([]string{}, card.Tags...),
		HarnessDefault: card.HarnessDefault,
		Liveness:       AgentLivenessOffline,
		Assignable:     card.Status != "retired",
	}
	defaultVariant := card.DefaultVariant()
	for _, session := range live {
		if session.Name != defaultVariant.SessionStem {
			continue
		}
		projection.Liveness = AgentLivenessLive
		projection.SessionID = session.Name
		projection.Status = session.Status
		projection.ContextPct = session.ContextPct
		projection.BeadID = session.BeadID
		projection.Attached = session.Attached
		return projection
	}
	return projection
}

func parsePersonaCard(expectedID string, raw []byte) (*PersonaCard, error) {
	parser := newPersonaParser(raw)
	schema := parser.schema
	if schema > CurrentSchema {
		return nil, fmt.Errorf("%w: schema %d", ErrUnsupportedSchema, schema)
	}
	card := &PersonaCard{
		Schema:         schema,
		ID:             parser.card["id"],
		DisplayName:    parser.card["display_name"],
		Kind:           parser.card["kind"],
		Summary:        parser.card["summary"],
		Status:         parser.card["status"],
		Tags:           parser.cardTags,
		HarnessDefault: parser.harness["default"],
		Notes:          parser.notes,
		ETag:           etag(raw),
		TOML:           string(raw),
	}
	if card.Schema == 0 {
		return nil, fmt.Errorf("%w: schema is required", ErrInvalidSlug)
	}
	if card.ID == "" || card.Kind == "" || card.HarnessDefault == "" {
		return nil, fmt.Errorf("%w: required persona fields missing", ErrInvalidSlug)
	}
	if expectedID != "" && card.ID != expectedID {
		return nil, fmt.Errorf("%w: filename id %q does not match card id %q", ErrInvalidSlug, expectedID, card.ID)
	}
	for _, variant := range parser.variants {
		if variant.ID == card.HarnessDefault && variant.SessionStem == "" {
			variant.SessionStem = card.ID
		}
		card.HarnessVariants = append(card.HarnessVariants, variant)
	}
	if len(card.HarnessVariants) == 0 {
		return nil, fmt.Errorf("%w: harness variant is required", ErrInvalidSlug)
	}
	return card, nil
}

type personaParser struct {
	schema    int
	card      map[string]string
	cardTags  []string
	harness   map[string]string
	variants  []HarnessVariant
	notes     []PersonaNote
	section   string
	variantIx int
	noteIx    int
}

func newPersonaParser(raw []byte) *personaParser {
	p := &personaParser{
		card:      map[string]string{},
		harness:   map[string]string{},
		variantIx: -1,
		noteIx:    -1,
	}
	for _, line := range splitLines(raw) {
		trimmed := strings.TrimSpace(line.body)
		switch trimmed {
		case "[card]":
			p.section = "card"
			continue
		case "[harness]":
			p.section = "harness"
			continue
		case "[[harness.variant]]":
			p.section = "harness.variant"
			p.variants = append(p.variants, HarnessVariant{})
			p.variantIx = len(p.variants) - 1
			continue
		case "[[note]]":
			p.section = "note"
			p.notes = append(p.notes, PersonaNote{})
			p.noteIx = len(p.notes) - 1
			continue
		}
		key, ok := topLevelKey(line.body)
		if !ok {
			continue
		}
		value := strings.TrimSpace(valuePart(line.body))
		switch p.section {
		case "":
			if key == "schema" {
				p.schema, _ = strconv.Atoi(value)
			}
		case "card":
			if key == "tags" {
				p.cardTags = parseStringList(value)
			} else {
				p.card[key] = parseString(value)
			}
		case "harness":
			p.harness[key] = parseString(value)
		case "harness.variant":
			if p.variantIx >= 0 {
				setVariantField(&p.variants[p.variantIx], key, parseString(value))
			}
		case "note":
			if p.noteIx >= 0 {
				setNoteField(&p.notes[p.noteIx], key, parseString(value))
			}
		}
	}
	return p
}

func setVariantField(v *HarnessVariant, key, value string) {
	switch key {
	case "id":
		v.ID = value
	case "session_stem":
		v.SessionStem = value
	case "launch":
		v.Launch = value
	case "source":
		v.Source = value
	}
}

func setNoteField(n *PersonaNote, key, value string) {
	switch key {
	case "ts":
		n.Timestamp = value
	case "actor":
		n.Actor = value
	case "text":
		n.Text = value
	}
}

func renderPersona(req CreatePersonaRequest, harness, sessionStem string, tags []string) string {
	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.ID
	}
	var b strings.Builder
	b.WriteString("schema = 1\n\n")
	b.WriteString("[card]\n")
	b.WriteString("id = " + renderString(req.ID) + "\n")
	b.WriteString("display_name = " + renderString(displayName) + "\n")
	b.WriteString("kind = " + renderString(req.Kind) + "\n")
	if req.Summary != "" {
		b.WriteString("summary = " + renderString(req.Summary) + "\n")
	}
	b.WriteString("tags = [" + renderStringList(tags) + "]\n")
	b.WriteString("status = \"active\"\n\n")
	b.WriteString("[harness]\n")
	b.WriteString("default = " + renderString(harness) + "\n\n")
	b.WriteString("[[harness.variant]]\n")
	b.WriteString("id = " + renderString(harness) + "\n")
	b.WriteString("session_stem = " + renderString(sessionStem) + "\n")
	if req.Launch != "" {
		b.WriteString("launch = " + renderString(req.Launch) + "\n")
	}
	if req.Source != "" {
		b.WriteString("source = " + renderString(req.Source) + "\n")
	}
	return b.String()
}

func appendHarnessVariant(raw string, variant HarnessVariant) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(raw, "\r\n"))
	b.WriteString("\n\n[[harness.variant]]\n")
	b.WriteString("id = " + renderString(variant.ID) + "\n")
	b.WriteString("session_stem = " + renderString(variant.SessionStem) + "\n")
	if variant.Launch != "" {
		b.WriteString("launch = " + renderString(variant.Launch) + "\n")
	}
	if variant.Source != "" {
		b.WriteString("source = " + renderString(variant.Source) + "\n")
	}
	return b.String()
}

func appendPersonaNote(raw string, note PersonaNote) string {
	var b strings.Builder
	b.WriteString(strings.TrimRight(raw, "\r\n"))
	b.WriteString("\n\n[[note]]\n")
	b.WriteString("ts = " + renderString(note.Timestamp) + "\n")
	b.WriteString("actor = " + renderString(note.Actor) + "\n")
	b.WriteString("text = " + renderString(note.Text) + "\n")
	return b.String()
}

func setSectionScalar(raw, section, key, value string) string {
	lines := splitLines([]byte(raw))
	inSection := false
	insertAt := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line.body)
		if trimmed == "["+section+"]" {
			inSection = true
			insertAt = i + 1
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "[") {
			insertAt = i
			break
		}
		if inSection {
			if field, ok := topLevelKey(line.body); ok {
				if field == key {
					lines[i].body = replaceScalarValue(line.body, value)
					return renderLines(lines)
				}
				insertAt = i + 1
			}
		}
	}
	newLine := tomlLine{body: key + " = " + value, newline: "\n"}
	lines = append(lines, tomlLine{})
	copy(lines[insertAt+1:], lines[insertAt:])
	lines[insertAt] = newLine
	return renderLines(lines)
}

func renderLines(lines []tomlLine) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(line.body)
		b.WriteString(line.newline)
	}
	return b.String()
}

func parseString(value string) string {
	value = strings.TrimSpace(value)
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return value
}

func parseStringList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		values = append(values, parseString(part))
	}
	return values
}

func renderStringList(values []string) string {
	rendered := make([]string, 0, len(values))
	for _, value := range values {
		rendered = append(rendered, renderString(value))
	}
	return strings.Join(rendered, ", ")
}

func normalizeTags(values []string) []string {
	tags := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				tags = appendUnique(tags, part)
			}
		}
	}
	return tags
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func removeValue(values []string, value string) []string {
	next := values[:0]
	for _, existing := range values {
		if existing != value {
			next = append(next, existing)
		}
	}
	return next
}

func hasBareCapability(tags []string, capability string) bool {
	if !isBareCapability(capability) {
		return false
	}
	for _, tag := range tags {
		if tag == capability && isBareCapability(tag) {
			return true
		}
	}
	return false
}

func isBareCapability(tag string) bool {
	return tag != "" && !strings.Contains(tag, ":")
}

func validatePersonaID(id string) error {
	if err := validateSlug(id); err != nil {
		return err
	}
	if id != strings.ToLower(id) || strings.Contains(id, ".") {
		return ErrInvalidSlug
	}
	if strings.HasPrefix(id, "-") || strings.HasSuffix(id, "-") {
		return ErrInvalidSlug
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return ErrInvalidSlug
	}
	return nil
}

func inferHarness(source string) string {
	source = strings.ToLower(source)
	switch {
	case strings.Contains(source, ".codex"):
		return "openai-codex"
	case strings.Contains(source, ".hermes/profiles"):
		return "hermes"
	default:
		return ""
	}
}

func inferLaunch(harness, source string) string {
	if harness == "openai-codex" {
		return "codex --yolo -c check_for_update_on_startup=false"
	}
	if harness == "claude-code" {
		return "claude --dangerously-skip-permissions --effort=\"max\""
	}
	if harness == "hermes" && source != "" {
		return "hermes --profile " + shellQuote(source)
	}
	return ""
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
