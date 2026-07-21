package formations

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type CanonicalRunProjection struct {
	view      RunView
	events    []projectedEvent
	latestSeq uint64
}

type projectedEvent struct {
	scanSeq uint64
	omitted bool
	safe    SafeRunEvent
}

type canonicalDocuments struct {
	byRole map[CanonicalInputRole][]CanonicalInputDocument
}

type rawProjectionEvent struct {
	envelope    safeEventEnvelope
	typeName    string
	data        map[string]json.RawMessage
	rawData     json.RawMessage
	writerFence uint64
}

const schema1ProjectionLedgerReadMaximumBytes = int64(64 << 20)

type schema1CanonicalRunReader struct {
	store *Store
}

func (reader *schema1CanonicalRunReader) ReadRun(runID string) (CanonicalRunReadInput, error) {
	if reader == nil || reader.store == nil || !validRunID(runID) {
		return CanonicalRunReadInput{}, ErrInvalidSlug
	}
	ledger, err := reader.store.openRunLedger(runID, false)
	if err != nil {
		return CanonicalRunReadInput{}, err
	}
	defer ledger.close()
	ledgerBytes, err := readSchema1ProjectionLedger(ledger.file)
	if err != nil {
		return CanonicalRunReadInput{}, err
	}
	class, classifyErr := classifyRuntimeAuthorityLedger(bytes.NewReader(ledgerBytes), runtimeAuthoritySchema, runID)
	if classifyErr != nil {
		if runtimeAuthorityClassifierRequiresAuthority(classifyErr) {
			return CanonicalRunReadInput{}, fmt.Errorf("%w: %w: classify canonical run", ErrRunLedgerInvalid, ErrRuntimeAuthorityNonAuthorizing)
		}
		return CanonicalRunReadInput{}, fmt.Errorf("%w: classify canonical run: %v", ErrRunLedgerInvalid, classifyErr)
	}
	if class != RuntimeAuthoritySchema1Inspection {
		return CanonicalRunReadInput{}, fmt.Errorf("%w: %w: schema-2 canonical reader unavailable", ErrRunLedgerInvalid, ErrRuntimeAuthorityNonAuthorizing)
	}
	snapshot, err := readRunArtifactAt(ledger.directory, runID+".snapshot.toml", runtimeAuthorityMaxRecordBytes)
	if err != nil {
		return CanonicalRunReadInput{}, fmt.Errorf("%w: read graph snapshot: %v", ErrRunLedgerInvalid, err)
	}
	bindings, err := readRunArtifactAt(ledger.directory, runID+".bindings.toml", runtimeAuthorityMaxRecordBytes)
	if err != nil {
		return CanonicalRunReadInput{}, fmt.Errorf("%w: read bindings snapshot: %v", ErrRunLedgerInvalid, err)
	}
	return CanonicalRunReadInput{
		RunID: runID, Source: CanonicalRunSourceSchema1,
		Documents: []CanonicalInputDocument{
			projectionDocument(CanonicalInputRoleSchema1Ledger, ledgerBytes),
			projectionDocument(CanonicalInputRoleSchema1GraphSnapshot, snapshot),
			projectionDocument(CanonicalInputRoleSchema1BindingsSnapshot, bindings),
		},
	}, nil
}

func projectionDocument(role CanonicalInputRole, raw []byte) CanonicalInputDocument {
	owned := append([]byte(nil), raw...)
	return CanonicalInputDocument{Role: role, Bytes: owned, SHA256: projectionSHA(owned)}
}

func readSchema1ProjectionLedger(file *os.File) ([]byte, error) {
	if file == nil {
		return nil, ErrRunLedgerInvalid
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() < 0 || info.Size() > schema1ProjectionLedgerReadMaximumBytes {
		return nil, projectionError(ErrRunProjectionResourceLimit, "run ledger exceeds read limit")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(file, schema1ProjectionLedgerReadMaximumBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > schema1ProjectionLedgerReadMaximumBytes {
		return nil, projectionError(ErrRunProjectionResourceLimit, "run ledger exceeds read limit")
	}
	return raw, nil
}

func (reader *schema1CanonicalRunReader) ListRunIdentities(request RunIdentityPageRequest) (RunIdentityPage, error) {
	if reader == nil || reader.store == nil || request.Limit < 1 || request.Limit > RunListPageLimit || (request.After != "" && !validRunID(request.After)) {
		return RunIdentityPage{}, projectionError(ErrRunProjectionInvalid, "invalid identity page selector")
	}
	runs, _, err := reader.store.openRunsDirectory(false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrNotFound) {
			return RunIdentityPage{RunIDs: []string{}, Cursor: request.After}, nil
		}
		return RunIdentityPage{}, err
	}
	defer runs.Close()
	candidates := make([]string, 0, request.Limit+1)
	seen := map[string]bool{}
	insert := func(runID string) {
		if seen[runID] || (request.After != "" && runID <= request.After) {
			return
		}
		seen[runID] = true
		position := sort.SearchStrings(candidates, runID)
		candidates = append(candidates, "")
		copy(candidates[position+1:], candidates[position:])
		candidates[position] = runID
		if len(candidates) > request.Limit+1 {
			delete(seen, candidates[len(candidates)-1])
			candidates = candidates[:len(candidates)-1]
		}
	}
	for {
		slugs, done, readErr := readRuntimeAuthorityDirectoryNameBatch(runs, runtimeAuthorityDirectoryBatchSize)
		if readErr != nil {
			return RunIdentityPage{}, readErr
		}
		for _, slug := range slugs {
			if validateSlug(slug) != nil {
				continue
			}
			directory, openErr := openRuntimeAuthorityDirectoryAt(runs, slug)
			if openErr != nil {
				if errors.Is(openErr, syscall.ELOOP) || errors.Is(openErr, syscall.ENOTDIR) || errors.Is(openErr, os.ErrNotExist) {
					continue
				}
				return RunIdentityPage{}, openErr
			}
			for {
				names, namesDone, namesErr := readRuntimeAuthorityDirectoryNameBatch(directory, runtimeAuthorityDirectoryBatchSize)
				if namesErr != nil {
					_ = directory.Close()
					return RunIdentityPage{}, namesErr
				}
				for _, name := range names {
					if strings.HasSuffix(name, ".ndjson") {
						runID := strings.TrimSuffix(name, ".ndjson")
						if validRunID(runID) {
							insert(runID)
						}
					}
				}
				if namesDone {
					break
				}
			}
			_ = directory.Close()
		}
		if done {
			break
		}
	}
	for _, runID := range candidates {
		ledger, resolveErr := reader.store.openRunLedger(runID, false)
		if resolveErr != nil {
			return RunIdentityPage{}, resolveErr
		}
		ledger.close()
	}
	hasMore := len(candidates) > request.Limit
	if hasMore {
		candidates = candidates[:request.Limit]
	}
	cursor := request.After
	if len(candidates) != 0 {
		cursor = candidates[len(candidates)-1]
	}
	return RunIdentityPage{RunIDs: candidates, Cursor: cursor, HasMore: hasMore}, nil
}

func (*schema1CanonicalRunReader) ReadCommand(SubmittedCommandIdentity) (CanonicalCommandReadInput, error) {
	return CanonicalCommandReadInput{}, projectionError(ErrRunCommandNotTerminal, "schema-1 has no command authority")
}

func projectionError(cause error, format string, args ...any) error {
	return fmt.Errorf("%w: %s", cause, fmt.Sprintf(format, args...))
}

func projectionSHA(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func projectionGeneration(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = io.WriteString(hash, strconv.Itoa(len(part)))
		_, _ = io.WriteString(hash, ":")
		_, _ = io.WriteString(hash, part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func firstLedgerRecord(raw []byte) []byte {
	for _, line := range bytes.Split(raw, []byte{'\n'}) {
		if trimmed := bytes.TrimSpace(line); len(trimmed) != 0 {
			return append([]byte(nil), trimmed...)
		}
	}
	return nil
}

func emptyToZero(value string) string { return value }

func ProjectCanonicalRun(input CanonicalRunReadInput) (CanonicalRunProjection, error) {
	documents, err := validateCanonicalDocuments(input)
	if err != nil {
		return CanonicalRunProjection{}, err
	}
	if input.Source == CanonicalRunSourceSchema1 {
		return projectSchema1Run(input.RunID, documents)
	}
	return projectSchema2Run(input.RunID, documents)
}

func validateCanonicalDocuments(input CanonicalRunReadInput) (canonicalDocuments, error) {
	if !validRunID(input.RunID) || (input.Source != CanonicalRunSourceSchema1 && input.Source != CanonicalRunSourceSchema2) {
		return canonicalDocuments{}, projectionError(ErrRunProjectionInvalid, "invalid canonical input identity")
	}
	result := canonicalDocuments{byRole: make(map[CanonicalInputRole][]CanonicalInputDocument)}
	for _, document := range input.Documents {
		copyBytes := append([]byte(nil), document.Bytes...)
		sum := sha256.Sum256(copyBytes)
		if document.SHA256 != hex.EncodeToString(sum[:]) {
			return canonicalDocuments{}, projectionError(ErrRunProjectionInvalid, "document hash mismatch")
		}
		copyDocument := CanonicalInputDocument{Role: document.Role, Bytes: copyBytes, SHA256: document.SHA256}
		result.byRole[document.Role] = append(result.byRole[document.Role], copyDocument)
	}
	if input.Source == CanonicalRunSourceSchema1 {
		allowed := map[CanonicalInputRole]bool{
			CanonicalInputRoleSchema1Ledger: true, CanonicalInputRoleSchema1GraphSnapshot: true, CanonicalInputRoleSchema1BindingsSnapshot: true,
		}
		for role, items := range result.byRole {
			if !allowed[role] || len(items) != 1 {
				return canonicalDocuments{}, projectionError(ErrRunProjectionInvalid, "invalid schema-1 role cardinality")
			}
		}
		for role := range allowed {
			if len(result.byRole[role]) != 1 {
				return canonicalDocuments{}, projectionError(ErrRunProjectionInvalid, "missing schema-1 role")
			}
		}
		return result, nil
	}

	singletons := map[CanonicalInputRole]bool{
		CanonicalInputRoleSchema2WorkspaceRegistry: true, CanonicalInputRoleSchema2WorkspaceBootstrap: true,
		CanonicalInputRoleSchema2WorkspaceAuthority: true, CanonicalInputRoleSchema2RunBootstrap: true,
		CanonicalInputRoleSchema2GraphSnapshot: true, CanonicalInputRoleSchema2PrivateBindings: true,
		CanonicalInputRoleSchema2Ledger: true,
	}
	multiple := map[CanonicalInputRole]bool{
		CanonicalInputRoleSchema2AdmissionPolicy: true, CanonicalInputRoleSchema2CommandRecord: true,
	}
	for role, items := range result.byRole {
		switch {
		case singletons[role]:
			if len(items) != 1 {
				return canonicalDocuments{}, projectionError(ErrRunProjectionInvalid, "invalid schema-2 singleton cardinality")
			}
		case multiple[role]:
			if len(items) == 0 {
				return canonicalDocuments{}, projectionError(ErrRunProjectionInvalid, "missing schema-2 record family")
			}
		case role == CanonicalInputRoleSchema2RunPrivateState:
			// I5 Option A: no nonempty private-state shape is authored in this slice.
			return canonicalDocuments{}, projectionError(ErrRunProjectionInvalid, "unreferenced run private state")
		default:
			return canonicalDocuments{}, projectionError(ErrRunProjectionInvalid, "unknown schema-2 role")
		}
	}
	for role := range singletons {
		if len(result.byRole[role]) != 1 {
			return canonicalDocuments{}, projectionError(ErrRunProjectionInvalid, "missing schema-2 singleton")
		}
	}
	for role := range multiple {
		if len(result.byRole[role]) == 0 {
			return canonicalDocuments{}, projectionError(ErrRunProjectionInvalid, "missing schema-2 record family")
		}
	}
	return result, nil
}

func ProjectRunView(projection CanonicalRunProjection) RunView {
	return cloneProjectionValue(reflect.ValueOf(projection.view)).Interface().(RunView)
}

func ProjectRunEventPage(projection CanonicalRunProjection, since uint64, limit int) (RunEventPage, error) {
	if since > MaxJSONSafeInteger || limit < 1 || limit > RunPageMaximumLimit {
		return RunEventPage{}, projectionError(ErrRunProjectionInvalid, "invalid event page selector")
	}
	page := RunEventPage{
		Schema: RunEventPageSchema, RunID: projection.view.RunID, Generation: projection.view.Generation,
		Source: projection.view.Source, Cursor: since, HasMore: projection.latestSeq > since, Events: []SafeRunEvent{},
	}
	scanned := 0
	for _, stored := range projection.events {
		if stored.scanSeq <= since {
			continue
		}
		if scanned == limit {
			break
		}
		candidateEvents := append([]SafeRunEvent(nil), page.Events...)
		if !stored.omitted && stored.safe != nil {
			candidateEvents = append(candidateEvents, cloneSafeRunEvent(stored.safe))
		}
		candidate := page
		candidate.Events = candidateEvents
		candidate.Cursor = stored.scanSeq
		candidate.HasMore = projection.latestSeq > stored.scanSeq
		encoded, err := json.Marshal(candidate)
		if err != nil {
			return RunEventPage{}, projectionError(ErrRunProjectionInvalid, "event page encode failed")
		}
		if len(encoded) > RunPageMaximumBytes {
			if !stored.omitted && stored.safe != nil && len(page.Events) == 0 {
				return RunEventPage{}, projectionError(ErrRunProjectionResourceLimit, "safe event exceeds page byte limit")
			}
			break
		}
		page = candidate
		scanned++
	}
	page.HasMore = projection.latestSeq > page.Cursor
	return page, nil
}

func cloneSafeRunEvent(event SafeRunEvent) SafeRunEvent {
	if event == nil {
		return nil
	}
	return cloneProjectionValue(reflect.ValueOf(event)).Interface().(SafeRunEvent)
}

func cloneProjectionValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneProjectionValue(value.Elem())
		result := reflect.New(value.Type()).Elem()
		result.Set(cloned)
		return result
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type().Elem())
		result.Elem().Set(cloneProjectionValue(value.Elem()))
		return result
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			result.Index(index).Set(cloneProjectionValue(value.Index(index)))
		}
		return result
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			result.SetMapIndex(cloneProjectionValue(iterator.Key()), cloneProjectionValue(iterator.Value()))
		}
		return result
	case reflect.Struct:
		result := reflect.New(value.Type()).Elem()
		result.Set(value)
		for index := 0; index < value.NumField(); index++ {
			if result.Field(index).CanSet() && value.Type().Field(index).PkgPath == "" {
				result.Field(index).Set(cloneProjectionValue(value.Field(index)))
			}
		}
		return result
	default:
		return value
	}
}

func validateProjectedEventSizes(projection CanonicalRunProjection) error {
	for _, event := range projection.events {
		if event.omitted || event.safe == nil {
			continue
		}
		page := RunEventPage{
			Schema: RunEventPageSchema, RunID: projection.view.RunID, Generation: projection.view.Generation,
			Source: projection.view.Source, Cursor: event.scanSeq, HasMore: false, Events: []SafeRunEvent{event.safe},
		}
		encoded, err := json.Marshal(page)
		if err != nil {
			return projectionError(ErrRunProjectionInvalid, "safe event encode failed")
		}
		if len(encoded) > RunPageMaximumBytes {
			return projectionError(ErrRunProjectionResourceLimit, "safe event exceeds page byte limit")
		}
	}
	return nil
}

func ProjectCommandReceipt(input CanonicalCommandReadInput) (RunCommandReceipt, error) {
	fail := func(format string, args ...any) (RunCommandReceipt, error) {
		return nil, projectionError(ErrRunCommandNotTerminal, format, args...)
	}
	if input.Source != CanonicalRunSourceSchema2 || !safeProjectionIdentifier(input.Submitted.CommandID) ||
		!validProjectionCommandKind(input.Submitted.CommandKind) || !lowercaseProjectionSHA(input.Submitted.CommandPayloadSHA256) {
		return fail("invalid submitted command identity")
	}
	object, err := decodeUniqueJSONObject(input.Record)
	if err != nil {
		return fail("invalid command record JSON")
	}
	state, err := requiredJSONString(object, "state")
	if err != nil || (state != "applied" && state != "rejected") {
		return fail("command is not terminal")
	}
	allowed := stringSet(
		"commandSchema", "recordRev", "priorGeneration", "commandEncoding", "commandId", "commandKind",
		"commandPayload", "commandPayloadSha256", "admittedWriterFence", "stateWriterFence", "state",
		"outcomeWriterFence", "decisionAdmissionPolicyRef",
	)
	if state == "applied" {
		allowed["runId"] = true
		allowed["effectSeq"] = true
	} else {
		allowed["rejectionCode"] = true
	}
	for key := range object {
		if !allowed[key] {
			return fail("unknown command record member")
		}
	}
	for key := range allowed {
		if _, ok := object[key]; !ok {
			return fail("missing command record member %s", key)
		}
	}
	commandSchema, schemaErr := requiredJSONSafeUint(object, "commandSchema")
	recordRev, revisionErr := requiredJSONSafeUint(object, "recordRev")
	commandID, idErr := requiredJSONString(object, "commandId")
	commandKind, kindErr := requiredJSONString(object, "commandKind")
	payloadSHA, payloadHashErr := requiredJSONString(object, "commandPayloadSha256")
	commandEncoding, encodingErr := requiredJSONString(object, "commandEncoding")
	admittedFence, admittedErr := requiredJSONSafeUint(object, "admittedWriterFence")
	stateFence, stateFenceErr := requiredJSONSafeUint(object, "stateWriterFence")
	outcomeFence, outcomeFenceErr := requiredJSONSafeUint(object, "outcomeWriterFence")
	if schemaErr != nil || revisionErr != nil || idErr != nil || kindErr != nil || payloadHashErr != nil || encodingErr != nil || admittedErr != nil || stateFenceErr != nil || outcomeFenceErr != nil ||
		commandSchema != 1 || recordRev != 2 || commandEncoding != "run-command-jcs-v1" || !validProjectionCommandKind(commandKind) ||
		commandID != input.Submitted.CommandID || commandKind != input.Submitted.CommandKind || payloadSHA != input.Submitted.CommandPayloadSHA256 ||
		!lowercaseProjectionSHA(payloadSHA) || admittedFence == 0 || stateFence < admittedFence || outcomeFence == 0 || outcomeFence > stateFence {
		return fail("command record identity mismatch")
	}
	payloadRaw, ok := object["commandPayload"]
	if !ok || projectionSHA(payloadRaw) != payloadSHA {
		return fail("command payload hash mismatch")
	}
	payloadObject, payloadErr := decodeUniqueJSONObject(payloadRaw)
	if payloadErr != nil {
		return fail("invalid command payload")
	}
	payloadKind, payloadKindErr := requiredJSONString(payloadObject, "kind")
	if payloadKindErr != nil || payloadKind != commandKind {
		return fail("command payload kind mismatch")
	}
	priorObject, priorErr := decodeUniqueJSONObject(object["priorGeneration"])
	if priorErr != nil {
		return fail("invalid command predecessor")
	}
	priorRev, priorRevErr := requiredJSONSafeUint(priorObject, "recordRev")
	priorSHA, priorSHAErr := requiredJSONString(priorObject, "sha256")
	if len(priorObject) != 2 || priorRevErr != nil || priorSHAErr != nil || priorRev != 1 || !lowercaseProjectionSHA(priorSHA) {
		return fail("invalid command predecessor")
	}
	pending := map[string]any{
		"commandSchema": uint64(1), "recordRev": uint64(1), "priorGeneration": nil,
		"commandEncoding": commandEncoding, "commandId": commandID, "commandKind": commandKind,
		"commandPayload": json.RawMessage(append([]byte(nil), payloadRaw...)), "commandPayloadSha256": payloadSHA,
		"admittedWriterFence": admittedFence, "stateWriterFence": admittedFence, "state": "pending",
	}
	pendingRaw, marshalErr := json.Marshal(pending)
	if marshalErr != nil || projectionSHA(pendingRaw) != priorSHA {
		return fail("command predecessor hash mismatch")
	}
	policy, policyErr := decodeDecisionPolicy(object["decisionAdmissionPolicyRef"], commandKind)
	if policyErr != nil {
		return fail("invalid decision policy")
	}
	if state == "applied" {
		runID, runErr := requiredJSONString(object, "runId")
		effectSeq, effectErr := requiredJSONSafeUint(object, "effectSeq")
		if runErr != nil || effectErr != nil || !validRunID(runID) || effectSeq == 0 {
			return fail("invalid applied command arm")
		}
		return RunCommandAppliedReceipt{
			CommandID: commandID, CommandPayloadSHA256: payloadSHA, CommandKind: commandKind,
			OutcomeWriterFence: strconv.FormatUint(outcomeFence, 10), State: state, RunID: runID,
			EffectSeq: effectSeq, DecisionAdmissionPolicyRef: policy,
		}, nil
	}
	rejectionCode, rejectionErr := requiredJSONString(object, "rejectionCode")
	if rejectionErr != nil || strings.TrimSpace(rejectionCode) == "" {
		return fail("invalid rejected command arm")
	}
	return RunCommandRejectedReceipt{
		CommandID: commandID, CommandPayloadSHA256: payloadSHA, CommandKind: commandKind,
		OutcomeWriterFence: strconv.FormatUint(outcomeFence, 10), State: state,
		RejectionCode: rejectionCode, DecisionAdmissionPolicyRef: policy,
	}, nil
}

func validProjectionCommandKind(kind string) bool {
	switch kind {
	case "start", "resume", "cancel", "verdict":
		return true
	default:
		return false
	}
}

func decodeDecisionPolicy(raw json.RawMessage, commandKind string) (*AdmissionPolicyRef, error) {
	if commandKind != "start" {
		if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, errors.New("non-start policy")
		}
		return nil, nil
	}
	object, err := decodeUniqueJSONObject(raw)
	if err != nil || len(object) != 2 {
		return nil, errors.New("invalid start policy")
	}
	revision, revisionErr := requiredJSONSafeUint(object, "policyRev")
	hash, hashErr := requiredJSONString(object, "policySha256")
	if revisionErr != nil || hashErr != nil || revision == 0 || !lowercaseProjectionSHA(hash) {
		return nil, errors.New("invalid start policy")
	}
	return &AdmissionPolicyRef{PolicyRev: revision, PolicySHA256: hash}, nil
}

func oneDocument(documents canonicalDocuments, role CanonicalInputRole) CanonicalInputDocument {
	return documents.byRole[role][0]
}

func decodeProjectionLedger(raw []byte, source CanonicalRunSource, runID string) ([]rawProjectionEvent, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), int(runtimeAuthorityMaxRecordBytes))
	var events []rawProjectionEvent
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		event, err := decodeProjectionEvent(line, source, runID)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, projectionError(ErrRunProjectionInvalid, "ledger scan failed")
	}
	if len(events) == 0 {
		return nil, projectionError(ErrRunProjectionInvalid, "empty ledger")
	}
	for index, event := range events {
		want := uint64(index + 1)
		if event.envelope.Seq != want || event.envelope.Seq > MaxJSONSafeInteger {
			return nil, projectionError(ErrRunProjectionInvalid, "noncanonical event sequence")
		}
	}
	return events, nil
}

func decodeProjectionEvent(raw []byte, source CanonicalRunSource, runID string) (rawProjectionEvent, error) {
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return rawProjectionEvent{}, projectionError(ErrRunProjectionInvalid, "invalid event JSON")
	}
	allowed := stringSet("ts", "runId", "seq", "type", "actor", "boardId", "boardRev", "missionId", "beadId", "nodeId", "slotId", "gateId", "edgeId", "epoch", "attempt", "data")
	if source == CanonicalRunSourceSchema2 {
		allowed["schema"] = true
		allowed["authoritySchema"] = true
		allowed["writerFence"] = true
	}
	for key := range object {
		if !allowed[key] {
			return rawProjectionEvent{}, projectionError(ErrRunEventUnknown, "unknown event envelope member")
		}
	}
	var envelope safeEventEnvelope
	if envelope.Timestamp, err = requiredJSONString(object, "ts"); err != nil {
		return rawProjectionEvent{}, projectionError(ErrRunEventUnknown, "invalid timestamp")
	}
	if envelope.RunID, err = requiredJSONString(object, "runId"); err != nil || envelope.RunID != runID {
		return rawProjectionEvent{}, projectionError(ErrRunProjectionInvalid, "run identity mismatch")
	}
	if envelope.Seq, err = requiredJSONSafeUint(object, "seq"); err != nil || envelope.Seq == 0 {
		return rawProjectionEvent{}, projectionError(ErrRunProjectionInvalid, "invalid event sequence")
	}
	typeName, err := requiredJSONString(object, "type")
	if err != nil {
		return rawProjectionEvent{}, projectionError(ErrRunEventUnknown, "invalid event type")
	}
	if envelope.Actor, err = requiredJSONString(object, "actor"); err != nil {
		return rawProjectionEvent{}, projectionError(ErrRunEventUnknown, "invalid actor")
	}
	for key, target := range map[string]*string{"boardId": &envelope.BoardID, "missionId": &envelope.MissionID, "beadId": &envelope.BeadID, "nodeId": &envelope.NodeID, "slotId": &envelope.SlotID, "gateId": &envelope.GateID, "edgeId": &envelope.EdgeID} {
		if value, ok := object[key]; ok {
			if err := json.Unmarshal(value, target); err != nil {
				return rawProjectionEvent{}, projectionError(ErrRunEventUnknown, "invalid event identity")
			}
		}
	}
	for key, target := range map[string]*uint64{"boardRev": &envelope.BoardRev, "epoch": &envelope.Epoch, "attempt": &envelope.Attempt} {
		if _, ok := object[key]; ok {
			value, parseErr := requiredJSONSafeUint(object, key)
			if parseErr != nil {
				return rawProjectionEvent{}, projectionError(ErrRunEventUnknown, "invalid event integer")
			}
			*target = value
		}
	}
	rawData, ok := object["data"]
	if !ok {
		return rawProjectionEvent{}, projectionError(ErrRunEventUnknown, "event data missing")
	}
	data, err := decodeUniqueJSONObject(rawData)
	if err != nil {
		return rawProjectionEvent{}, projectionError(ErrRunEventUnknown, "invalid event data")
	}
	result := rawProjectionEvent{envelope: envelope, typeName: typeName, data: data, rawData: append([]byte(nil), rawData...)}
	if source == CanonicalRunSourceSchema2 {
		if schema, e := requiredJSONSafeUint(object, "schema"); e != nil || schema != 2 {
			return rawProjectionEvent{}, projectionError(ErrRunProjectionInvalid, "schema-2 event schema mismatch")
		}
		if authority, e := requiredJSONSafeUint(object, "authoritySchema"); e != nil || authority != 2 {
			return rawProjectionEvent{}, projectionError(ErrRunProjectionInvalid, "schema-2 authority mismatch")
		}
		if result.writerFence, err = requiredJSONSafeUint(object, "writerFence"); err != nil || result.writerFence == 0 {
			return rawProjectionEvent{}, projectionError(ErrRunProjectionInvalid, "invalid writer fence")
		}
	}
	return result, nil
}

func stringSet(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func decodeUniqueJSONObject(raw []byte) (map[string]json.RawMessage, error) {
	if err := rejectDuplicateJSONMembers(raw); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var result map[string]json.RawMessage
	if err := decoder.Decode(&result); err != nil || result == nil {
		return nil, errors.New("not a JSON object")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	return result, nil
}

func rejectDuplicateJSONMembers(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func(json.Token) error
	walk = func(token json.Token) error {
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok || seen[key] {
					return errors.New("duplicate JSON member")
				}
				seen[key] = true
				value, err := decoder.Token()
				if err != nil {
					return err
				}
				if err := walk(value); err != nil {
					return err
				}
			}
			_, err := decoder.Token()
			return err
		case '[':
			for decoder.More() {
				value, err := decoder.Token()
				if err != nil {
					return err
				}
				if err := walk(value); err != nil {
					return err
				}
			}
			_, err := decoder.Token()
			return err
		default:
			return errors.New("unexpected JSON delimiter")
		}
	}
	first, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := walk(first); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

func requiredJSONString(object map[string]json.RawMessage, key string) (string, error) {
	raw, ok := object[key]
	if !ok {
		return "", errors.New("missing string")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func requiredJSONSafeUint(object map[string]json.RawMessage, key string) (uint64, error) {
	raw, ok := object[key]
	if !ok {
		return 0, errors.New("missing integer")
	}
	if len(raw) == 0 || raw[0] == '-' || bytes.ContainsAny(raw, ".eE\"") {
		return 0, errors.New("not unsigned integer")
	}
	value, err := strconv.ParseUint(string(raw), 10, 64)
	if err != nil || value > MaxJSONSafeInteger {
		return 0, errors.New("not JSON-safe")
	}
	return value, nil
}

func validateDataKeys(data map[string]json.RawMessage, allowed, private map[string]bool) (map[string]json.RawMessage, error) {
	result := make(map[string]json.RawMessage, len(data))
	for key, value := range data {
		switch {
		case allowed[key]:
			result[key] = append([]byte(nil), value...)
		case private[key]:
		default:
			return nil, projectionError(ErrRunEventUnknown, "unknown event data member")
		}
	}
	return result, nil
}

func decodeTypedData[T any](data map[string]json.RawMessage) (T, error) {
	var result T
	raw, err := json.Marshal(data)
	if err != nil {
		return result, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

func eventEnvelope(event rawProjectionEvent) safeEventEnvelope { return event.envelope }

func sanitizeSchema1Event(event rawProjectionEvent) (SafeRunEvent, error) {
	allowed, private := schema1DataContract(event.typeName)
	if allowed == nil {
		return nil, projectionError(ErrRunEventUnknown, "unknown schema-1 event type")
	}
	data, err := validateDataKeys(event.data, allowed, private)
	if err != nil {
		return nil, err
	}
	envelope := eventEnvelope(event)
	switch event.typeName {
	case RunEventStarted:
		common := stringSet("boardSlug", "boardRev", "missionId", "beadId", "limits")
		for key := range common {
			if _, ok := data[key]; !ok {
				return nil, projectionError(ErrRunEventUnknown, "incomplete run_started")
			}
		}
		mode, hasMode := data["mode"]
		formationRaw, hasFormation := data["formationId"]
		if hasMode != hasFormation {
			return nil, projectionError(ErrRunEventUnknown, "invalid run root discriminant")
		}
		if !hasMode {
			var decoded SafeSchema1RunStartedMissionData
			if decoded, err = decodeTypedData[SafeSchema1RunStartedMissionData](data); err != nil {
				return nil, projectionError(ErrRunEventUnknown, "invalid mission start")
			}
			return SafeSchema1RunStartedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
		}
		var modeValue, formationID string
		if json.Unmarshal(mode, &modeValue) != nil || json.Unmarshal(formationRaw, &formationID) != nil || modeValue != "formation" || !safeProjectionIdentifier(formationID) || envelope.MissionID != "single_"+formationID {
			return nil, projectionError(ErrRunEventUnknown, "invalid formation start")
		}
		decoded, decodeErr := decodeTypedData[SafeSchema1RunStartedFormationData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid formation start")
		}
		return SafeSchema1RunStartedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventResumed:
		decoded, decodeErr := decodeSchema1RunResumedData(data)
		if decodeErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrRunEventUnknown, decodeErr)
		}
		return SafeSchema1RunResumedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventNodeWaiting:
		decoded, decodeErr := decodeTypedData[SafeSchema1NodeWaitingData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid node_waiting")
		}
		return SafeSchema1NodeWaitingEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventNodeStarted:
		if raw, ok := data["inputRefs"]; ok {
			clean, e := sanitizeInputIdentityArray(raw)
			if e != nil {
				return nil, e
			}
			data["inputRefs"] = clean
		}
		decoded, decodeErr := decodeTypedData[SafeSchema1NodeStartedData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid node_started")
		}
		return SafeSchema1NodeStartedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventOrchestrationTeam:
		if raw, ok := data["controller"]; ok {
			clean, e := sanitizeParticipant(raw)
			if e != nil {
				return nil, e
			}
			data["controller"] = clean
		}
		if raw, ok := data["workers"]; ok {
			clean, e := sanitizeParticipantArray(raw)
			if e != nil {
				return nil, e
			}
			data["workers"] = clean
		}
		decoded, decodeErr := decodeTypedData[SafeSchema1OrchestrationTeamData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid orchestration_team")
		}
		return SafeSchema1OrchestrationTeamEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventPeerPlane:
		if raw, ok := data["peers"]; ok {
			clean, e := sanitizeParticipantArray(raw)
			if e != nil {
				return nil, e
			}
			data["peers"] = clean
		}
		decoded, decodeErr := decodeTypedData[SafeSchema1PeerPlaneData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid peer_plane")
		}
		return SafeSchema1PeerPlaneEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventSlotDispatch:
		decoded, decodeErr := decodeTypedData[SafeSchema1SlotDispatchData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid slot_dispatch")
		}
		return SafeSchema1SlotDispatchEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventAdapterSend:
		decoded, decodeErr := decodeTypedData[SafeSchema1AdapterSendData](data)
		if decodeErr != nil || decoded.Adapter != "tmux" {
			return nil, projectionError(ErrRunEventUnknown, "invalid adapter_send")
		}
		return SafeSchema1AdapterSendEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventSlotResult:
		if raw, ok := data["sentinel"]; ok {
			clean, e := sanitizeClosedObject(raw, stringSet("runId", "status"), stringSet("artifact"))
			if e != nil {
				return nil, e
			}
			data["sentinel"] = clean
		}
		decoded, decodeErr := decodeTypedData[SafeSchema1SlotResultData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid slot_result")
		}
		return SafeSchema1SlotResultEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventNodeOutput:
		if raw, ok := data["outputs"]; ok {
			clean, e := sanitizeSchema1Outputs(raw)
			if e != nil {
				return nil, e
			}
			data["outputs"] = clean
		}
		decoded, decodeErr := decodeTypedData[SafeSchema1NodeOutputData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid node_output")
		}
		return SafeSchema1NodeOutputEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventGateEvaluating:
		if raw, ok := data["inputRef"]; ok {
			clean, e := sanitizeInputIdentity(raw)
			if e != nil {
				return nil, e
			}
			data["inputRef"] = clean
		}
		decoded, decodeErr := decodeTypedData[SafeSchema1GateEvaluatingData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid gate_evaluating")
		}
		if decoded.JudgeChain == nil {
			decoded.JudgeChain = []string{}
		}
		return SafeSchema1GateEvaluatingEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventGateVerdict:
		if raw, ok := data["inputRef"]; ok {
			clean, e := sanitizeInputIdentity(raw)
			if e != nil {
				return nil, e
			}
			data["inputRef"] = clean
		}
		decoded, decodeErr := decodeTypedData[SafeSchema1GateVerdictData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid gate_verdict")
		}
		if decoded.RoutedEdges == nil {
			decoded.RoutedEdges = []string{}
		}
		return SafeSchema1GateVerdictEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventVerificationVerdict:
		decoded, decodeErr := decodeTypedData[SafeSchema1VerificationVerdictData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid verification_verdict")
		}
		return SafeSchema1VerificationVerdictEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventEscalationRaised:
		decoded, decodeErr := decodeTypedData[SafeSchema1EscalationRaisedData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid escalation")
		}
		return SafeSchema1EscalationRaisedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventHumanInputRequested:
		if raw, ok := data["inputRef"]; ok {
			clean, e := sanitizeInputIdentity(raw)
			if e != nil {
				return nil, e
			}
			data["inputRef"] = clean
		}
		decoded, decodeErr := decodeTypedData[SafeSchema1HumanInputRequestedData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid human request")
		}
		return SafeSchema1HumanInputRequestedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventHumanVerdictRecorded:
		decoded, decodeErr := decodeTypedData[SafeSchema1HumanVerdictRecordedData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid human verdict")
		}
		return SafeSchema1HumanVerdictRecordedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventError:
		decoded, decodeErr := decodeTypedData[SafeSchema1ErrorData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid error event")
		}
		return SafeSchema1ErrorEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventBlocked:
		decoded, decodeErr := decodeSchema1RunBlockedData(data)
		if decodeErr != nil {
			return nil, fmt.Errorf("%w: %w", ErrRunEventUnknown, decodeErr)
		}
		return SafeSchema1RunBlockedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventCanceled:
		decoded, decodeErr := decodeTypedData[SafeSchema1RunCanceledData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid run_canceled")
		}
		return SafeSchema1RunCanceledEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventFailed:
		decoded, decodeErr := decodeTypedData[SafeSchema1RunFailedData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid run_failed")
		}
		return SafeSchema1RunFailedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case RunEventSucceeded:
		decoded, decodeErr := decodeTypedData[SafeSchema1RunSucceededData](data)
		if decodeErr != nil {
			return nil, projectionError(ErrRunEventUnknown, "invalid run_succeeded")
		}
		return SafeSchema1RunSucceededEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	default:
		return nil, projectionError(ErrRunEventUnknown, "unknown schema-1 event type")
	}
}

func schema1DataContract(eventType string) (map[string]bool, map[string]bool) {
	switch eventType {
	case RunEventStarted:
		return stringSet("boardSlug", "boardRev", "missionId", "beadId", "limits", "mode", "formationId"), stringSet("boardPath", "snapshot", "bindingsSnapshot", "objective")
	case RunEventResumed:
		return stringSet("resumedFromSeq", "resumedBy", "resumeMode", "reason", "openDispatches"), nil
	case RunEventNodeWaiting:
		return stringSet("neededInputs", "readyInputs", "totalInputs", "waitingFor"), nil
	case RunEventNodeStarted:
		return stringSet("nodeKind", "inputRefs", "reason"), stringSet("brief")
	case RunEventOrchestrationTeam:
		return stringSet("mode", "controllerSlot", "controller", "workers"), stringSet("socket", "cwd")
	case RunEventPeerPlane:
		return stringSet("mode", "peers"), stringSet("path", "socket", "cwd")
	case RunEventSlotDispatch:
		return stringSet("dispatchId", "nodeId", "slotId", "agentId", "harness", "phase", "promptSha256", "nativeAck", "recordedBeforeSend"), stringSet("sessionStem", "sessionRef", "promptRef")
	case RunEventAdapterSend:
		return stringSet("adapter", "dispatchId", "nodeId", "slotId", "phase", "socketSha256", "promptSha256", "sent"), stringSet("sessionRef")
	case RunEventSlotResult:
		return stringSet("dispatchId", "nodeId", "slotId", "status", "sentinel"), nil
	case RunEventNodeOutput:
		return stringSet("status", "text", "outputs", "reason"), stringSet("reportRef")
	case RunEventGateEvaluating:
		return stringSet("kinds", "criterion", "inputRef", "judgeChain"), nil
	case RunEventGateVerdict:
		return stringSet("verdict", "perKind", "routePort", "routedEdges", "reason", "inputRef"), nil
	case RunEventVerificationVerdict:
		return stringSet("verificationId", "verdict"), nil
	case RunEventEscalationRaised:
		return stringSet("trigger", "severity", "reason", "source", "nodeId", "gateId", "blocks"), nil
	case RunEventHumanInputRequested:
		return stringSet("gateId", "nodeId", "choices", "requestedBy", "inputRef", "codeVerdict", "codeReason", "codePerKind", "timeoutSeconds"), stringSet("prompt")
	case RunEventHumanVerdictRecorded:
		return stringSet("gateId", "nodeId", "verdict", "reason", "requestedSeq", "decidedBy"), nil
	case RunEventError:
		return stringSet("code", "message", "reason", "boundary", "nodeId", "gateId", "slotId", "dispatchId", "recoverable", "relatedSeq"), nil
	case RunEventBlocked:
		return stringSet("reason", "code", "boundary", "blockedNodeId", "blockedGateId", "waitingNodes", "recoverable", "resumeAllowed", "resumePolicy", "openDispatches", "nextEpoch"), nil
	case RunEventCanceled:
		return stringSet("reason", "requestedBy", "softInterruptedSlots", "final"), nil
	case RunEventFailed:
		return stringSet("code", "reason", "boundary", "recoverable", "relatedSeq", "final"), nil
	case RunEventSucceeded:
		return stringSet("final", "mode", "formationId", "missionId", "reason"), stringSet("summaryRef", "outputRefs", "artifactRefs")
	default:
		return nil, nil
	}
}

func sanitizeClosedObject(raw json.RawMessage, allowed, private map[string]bool) (json.RawMessage, error) {
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return nil, projectionError(ErrRunEventUnknown, "invalid nested object")
	}
	clean, err := validateDataKeys(object, allowed, private)
	if err != nil {
		return nil, err
	}
	return json.Marshal(clean)
}

func sanitizeInputIdentity(raw json.RawMessage) (json.RawMessage, error) {
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return nil, projectionError(ErrRunEventUnknown, "invalid input identity")
	}
	for _, key := range []string{"ref", "text", "reportRef", "artifactRef"} {
		delete(object, key)
	}
	clean, err := json.Marshal(object)
	if err != nil {
		return nil, projectionError(ErrRunEventUnknown, "invalid input identity")
	}
	value, err := decodeSafeInputIdentityValue(clean)
	if err != nil {
		return nil, projectionError(ErrRunEventUnknown, "invalid input identity")
	}
	return json.Marshal(value)
}

func sanitizeInputIdentityArray(raw json.RawMessage) (json.RawMessage, error) {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil, projectionError(ErrRunEventUnknown, "invalid input refs")
	}
	clean := make([]json.RawMessage, len(items))
	for index, item := range items {
		value, err := sanitizeInputIdentity(item)
		if err != nil {
			return nil, err
		}
		clean[index] = value
	}
	return json.Marshal(clean)
}

func sanitizeParticipant(raw json.RawMessage) (json.RawMessage, error) {
	return sanitizeClosedObject(raw, stringSet("slotId", "label", "agentId", "harness"), stringSet("sessionStem", "sessionRef"))
}

func sanitizeParticipantArray(raw json.RawMessage) (json.RawMessage, error) {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil, projectionError(ErrRunEventUnknown, "invalid participants")
	}
	clean := make([]json.RawMessage, len(items))
	for index, item := range items {
		value, err := sanitizeParticipant(item)
		if err != nil {
			return nil, err
		}
		clean[index] = value
	}
	return json.Marshal(clean)
}

func sanitizeSchema1Outputs(raw json.RawMessage) (json.RawMessage, error) {
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return nil, projectionError(ErrRunEventUnknown, "invalid outputs")
	}
	result := make(map[string]json.RawMessage, len(object))
	for portID, value := range object {
		if !safeProjectionIdentifier(portID) {
			return nil, projectionError(ErrRunEventUnknown, "invalid output port")
		}
		clean, cleanErr := sanitizeClosedObject(value, stringSet("text"), stringSet("ref", "reportRef", "artifactRef"))
		if cleanErr != nil {
			return nil, cleanErr
		}
		result[portID] = clean
	}
	return json.Marshal(result)
}

func decodeSchema1OpenDispatches(raw json.RawMessage) ([]SafeSchema1OpenDispatch, error) {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil, projectionError(ErrRunProjectionInvalid, "invalid open dispatches")
	}
	result := make([]SafeSchema1OpenDispatch, len(items))
	seen := map[string]bool{}
	for index, item := range items {
		object, err := decodeUniqueJSONObject(item)
		if err != nil {
			return nil, projectionError(ErrRunProjectionInvalid, "invalid open dispatch")
		}
		for key := range object {
			if !stringSet("dispatchId", "nodeId", "slotId", "dispatchSeq")[key] {
				return nil, projectionError(ErrRunProjectionInvalid, "unknown open dispatch member")
			}
		}
		if result[index].DispatchID, err = requiredJSONString(object, "dispatchId"); err != nil || !safeProjectionIdentifier(result[index].DispatchID) {
			return nil, projectionError(ErrRunProjectionInvalid, "invalid dispatch id")
		}
		if result[index].NodeID, err = requiredJSONString(object, "nodeId"); err != nil || !safeProjectionIdentifier(result[index].NodeID) {
			return nil, projectionError(ErrRunProjectionInvalid, "invalid node id")
		}
		if result[index].SlotID, err = requiredJSONString(object, "slotId"); err != nil || !safeProjectionIdentifier(result[index].SlotID) {
			return nil, projectionError(ErrRunProjectionInvalid, "invalid slot id")
		}
		if seen[result[index].DispatchID] {
			return nil, projectionError(ErrRunProjectionInvalid, "duplicate dispatch id")
		}
		seen[result[index].DispatchID] = true
		if _, ok := object["dispatchSeq"]; ok {
			value, e := requiredJSONSafeUint(object, "dispatchSeq")
			if e != nil {
				return nil, projectionError(ErrRunProjectionInvalid, "invalid dispatch sequence")
			}
			result[index].DispatchSeq = &value
		}
	}
	return result, nil
}

func decodeSchema1RunBlockedData(data map[string]json.RawMessage) (SafeSchema1RunBlockedData, error) {
	var result SafeSchema1RunBlockedData
	raw, ok := data["openDispatches"]
	if !ok {
		return result, projectionError(ErrRunProjectionInvalid, "missing open dispatches")
	}
	dispatches, err := decodeSchema1OpenDispatches(raw)
	if err != nil {
		return result, err
	}
	copyData := cloneRawMap(data)
	copyData["openDispatches"], _ = json.Marshal(dispatches)
	result, err = decodeTypedData[SafeSchema1RunBlockedData](copyData)
	if err != nil {
		return result, projectionError(ErrRunProjectionInvalid, "invalid run_blocked")
	}
	return result, nil
}

func decodeSchema1RunResumedData(data map[string]json.RawMessage) (SafeSchema1RunResumedData, error) {
	var result SafeSchema1RunResumedData
	raw, ok := data["openDispatches"]
	if !ok {
		return result, projectionError(ErrRunProjectionInvalid, "missing open dispatches")
	}
	dispatches, err := decodeSchema1OpenDispatches(raw)
	if err != nil {
		return result, err
	}
	copyData := cloneRawMap(data)
	copyData["openDispatches"], _ = json.Marshal(dispatches)
	result, err = decodeTypedData[SafeSchema1RunResumedData](copyData)
	if err != nil {
		return result, projectionError(ErrRunProjectionInvalid, "invalid run_resumed")
	}
	return result, nil
}

func cloneRawMap(input map[string]json.RawMessage) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage, len(input))
	for key, value := range input {
		result[key] = append([]byte(nil), value...)
	}
	return result
}

func safeProjectionIdentifier(value string) bool {
	return value != "" && runtimeAuthorityPathComponent(value) && !strings.Contains(value, "..")
}

func decodeClosedProjection[T any](raw json.RawMessage, allowed, required map[string]bool) (T, error) {
	var result T
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return result, err
	}
	for key := range object {
		if !allowed[key] {
			return result, errors.New("unknown member")
		}
	}
	for key := range required {
		if _, ok := object[key]; !ok {
			return result, errors.New("missing member")
		}
	}
	clean, err := json.Marshal(object)
	if err != nil {
		return result, err
	}
	decoder := json.NewDecoder(bytes.NewReader(clean))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

func closedProjectionArray[T any](raw json.RawMessage, decode func(json.RawMessage) (T, error)) ([]T, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		return nil, errors.New("invalid array")
	}
	result := make([]T, len(items))
	for index, item := range items {
		value, err := decode(item)
		if err != nil {
			return nil, err
		}
		result[index] = value
	}
	return result, nil
}

func projectionValueIn(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func projectionSafeSequence(value uint64) bool {
	return value > 0 && value <= MaxJSONSafeInteger
}

func projectionAscendingUnique(values []uint64, allowEmpty bool) bool {
	if !allowEmpty && len(values) == 0 {
		return false
	}
	var prior uint64
	for index, value := range values {
		if !projectionSafeSequence(value) || (index > 0 && value <= prior) {
			return false
		}
		prior = value
	}
	return true
}

func decodeSafeGateFeedback(raw json.RawMessage) (SafeGateFeedbackPayload, error) {
	allowed := stringSet("feedbackId", "gateId", "verdict", "evaluatedInput", "reason", "evidence", "gateSeq", "gateAttempt")
	result, err := decodeClosedProjection[SafeGateFeedbackPayload](raw, allowed, allowed)
	if err != nil || !safeProjectionIdentifier(result.FeedbackID) || !safeProjectionIdentifier(result.GateID) || result.Verdict != "fail" || !safeProjectionIdentifier(result.EvaluatedInput.InputID) || !projectionSafeSequence(result.EvaluatedInput.GateInputSeq) || !projectionSafeSequence(result.GateSeq) || !projectionSafeSequence(result.GateAttempt) || len(result.Reason) > 64<<10 {
		return SafeGateFeedbackPayload{}, errors.New("invalid gate feedback")
	}
	if err := validateSafeGateEvidence(result.Evidence); err != nil {
		return SafeGateFeedbackPayload{}, err
	}
	return result, nil
}

func decodePayloadProjection(raw json.RawMessage) (PayloadProjection, error) {
	outerAllowed := stringSet("availability", "exact", "classification", "sourceKind", "encoding", "mediaType", "sha256", "payload")
	outerRequired := stringSet("availability", "exact", "payload")
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return PayloadProjection{}, err
	}
	for key := range object {
		if !outerAllowed[key] {
			return PayloadProjection{}, errors.New("unknown payload projection member")
		}
	}
	for key := range outerRequired {
		if _, ok := object[key]; !ok {
			return PayloadProjection{}, errors.New("missing payload projection member")
		}
	}
	payloadObject, err := decodeUniqueJSONObject(object["payload"])
	if err != nil {
		return PayloadProjection{}, err
	}
	kind, err := requiredJSONString(payloadObject, "kind")
	if err != nil {
		return PayloadProjection{}, err
	}
	var payloadAllowed map[string]bool
	switch kind {
	case "work":
		payloadAllowed = stringSet("kind", "mediaType", "text")
	case "unavailable", "error":
		payloadAllowed = stringSet("kind", "code", "message", "retryable")
	case "gate_feedback":
		payloadAllowed = stringSet("kind", "feedback")
	default:
		return PayloadProjection{}, errors.New("invalid payload kind")
	}
	for key := range payloadObject {
		if !payloadAllowed[key] {
			return PayloadProjection{}, errors.New("unknown payload member")
		}
	}
	for key := range payloadAllowed {
		if _, ok := payloadObject[key]; !ok {
			return PayloadProjection{}, errors.New("missing payload member")
		}
	}
	if kind == "gate_feedback" {
		if _, err := decodeSafeGateFeedback(payloadObject["feedback"]); err != nil {
			return PayloadProjection{}, err
		}
	}
	clean, err := json.Marshal(object)
	if err != nil {
		return PayloadProjection{}, err
	}
	var result PayloadProjection
	decoder := json.NewDecoder(bytes.NewReader(clean))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil || result.Availability == "" {
		return PayloadProjection{}, errors.New("invalid payload projection")
	}
	if kind == "work" && (result.Payload.MediaType == "" || result.Payload.Text == "") {
		return PayloadProjection{}, errors.New("invalid work payload")
	}
	if (kind == "unavailable" || kind == "error") && (result.Payload.Code == "" || result.Payload.Message == "" || result.Payload.Retryable == nil) {
		return PayloadProjection{}, errors.New("invalid unavailable payload")
	}
	return result, nil
}

func decodePayloadProjections(raw json.RawMessage) (SafePayloadProjections, error) {
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return nil, err
	}
	result := make(SafePayloadProjections, len(object))
	for portID, item := range object {
		if !safeProjectionIdentifier(portID) {
			return nil, errors.New("invalid payload projection key")
		}
		projection, err := decodePayloadProjection(item)
		if err != nil {
			return nil, err
		}
		result[portID] = projection
	}
	return result, nil
}

func decodeProjectionHashes(raw json.RawMessage) (SafeProjectionHashes, error) {
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return nil, err
	}
	result := make(SafeProjectionHashes, len(object))
	for key, item := range object {
		var hash string
		if !safeProjectionIdentifier(key) || json.Unmarshal(item, &hash) != nil || !lowercaseProjectionSHA(hash) {
			return nil, errors.New("invalid projection hash")
		}
		result[key] = hash
	}
	return result, nil
}

func validateSafeGateEvidence(evidence []SafeGateEvidence) error {
	if evidence == nil {
		return errors.New("missing evidence")
	}
	for _, item := range evidence {
		switch item.Kind {
		case "artifact":
			if !safeProjectionIdentifier(item.ArtifactID) || item.Seq != 0 || item.Text != "" {
				return errors.New("invalid artifact evidence")
			}
		case "ledger":
			if !projectionSafeSequence(item.Seq) || item.ArtifactID != "" || item.Text != "" {
				return errors.New("invalid ledger evidence")
			}
		case "text":
			if item.Text == "" || len(item.Text) > 64<<10 || item.ArtifactID != "" || item.Seq != 0 {
				return errors.New("invalid text evidence")
			}
		default:
			return errors.New("invalid evidence kind")
		}
	}
	return nil
}

func decodeSafeInputIdentityValue(raw json.RawMessage) (SafeInputIdentity, error) {
	allowed := stringSet("edgeId", "originEdgeId", "deliveryEdgeId", "fromNodeId", "fromPortId", "sourceNodeId", "sourcePortId", "sourceOutputSeq", "sourceAttempt", "toPortId", "outputSeq", "inputId", "sourceKind", "runId", "seedId", "seedEncoding", "seedMediaType", "seedSha256", "toNodeId", "gateInputSeq", "payloadProjection")
	result, err := decodeClosedProjection[SafeInputIdentity](raw, allowed, nil)
	if err != nil {
		return SafeInputIdentity{}, err
	}
	for _, value := range []string{result.EdgeID, result.OriginEdgeID, result.DeliveryEdgeID, result.FromNodeID, result.FromPortID, result.SourceNodeID, result.SourcePortID, result.InputID, result.RunID, result.SeedID, result.ToNodeID, result.ToPortID} {
		if value != "" && !safeProjectionIdentifier(value) {
			return SafeInputIdentity{}, errors.New("invalid input identity")
		}
	}
	for _, value := range []uint64{result.OutputSeq, result.SourceOutputSeq, result.SourceAttempt, result.GateInputSeq} {
		if value > MaxJSONSafeInteger {
			return SafeInputIdentity{}, errors.New("invalid input sequence")
		}
	}
	if result.SeedSHA256 != "" && !lowercaseProjectionSHA(result.SeedSHA256) {
		return SafeInputIdentity{}, errors.New("invalid seed hash")
	}
	if result.PayloadProjection != nil {
		projection, err := decodePayloadProjection(rawObjectMember(raw, "payloadProjection"))
		if err != nil {
			return SafeInputIdentity{}, err
		}
		result.PayloadProjection = &projection
	}
	return result, nil
}

func rawObjectMember(raw json.RawMessage, key string) json.RawMessage {
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return nil
	}
	return object[key]
}

func canonicalProjectionJSON(raw json.RawMessage) ([]byte, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func validateRawProjectionHashes(outputsRaw json.RawMessage, hashes SafeProjectionHashes) error {
	outputs, err := decodeUniqueJSONObject(outputsRaw)
	if err != nil || len(outputs) != len(hashes) {
		return errors.New("projection hash key mismatch")
	}
	for key, output := range outputs {
		canonical, err := canonicalProjectionJSON(output)
		if err != nil || hashes[key] != projectionSHA(canonical) {
			return errors.New("projection hash mismatch")
		}
	}
	return nil
}

func replaceProjectionData(data map[string]json.RawMessage, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	data[key] = raw
	return nil
}

func decodeTurnInputs(raw json.RawMessage) (SafeTurnInputs, error) {
	allowed := stringSet("nodeStartedSeq", "priorTurnResults")
	result, err := decodeClosedProjection[SafeTurnInputs](raw, allowed, allowed)
	if err != nil || !projectionSafeSequence(result.NodeStartedSeq) || result.PriorTurnResults == nil {
		return SafeTurnInputs{}, errors.New("invalid turn inputs")
	}
	var prior uint64
	for index, item := range result.PriorTurnResults {
		if !projectionSafeSequence(item.SlotResultSeq) || !lowercaseProjectionSHA(item.TurnResultSHA256) || (index > 0 && item.SlotResultSeq <= prior) {
			return SafeTurnInputs{}, errors.New("invalid prior turn result")
		}
		prior = item.SlotResultSeq
	}
	return result, nil
}

func decodeEventTiming(raw json.RawMessage) (SafeEventTiming, error) {
	allowed := stringSet("startedAt", "finishedAt", "durationMs")
	result, err := decodeClosedProjection[SafeEventTiming](raw, allowed, allowed)
	if err != nil || result.StartedAt == "" || result.FinishedAt == "" || result.DurationMS > MaxJSONSafeInteger {
		return SafeEventTiming{}, errors.New("invalid timing")
	}
	return result, nil
}

func decodeRetryTargets(raw json.RawMessage) ([]SafeRetryTarget, error) {
	result, err := closedProjectionArray(raw, func(item json.RawMessage) (SafeRetryTarget, error) {
		allowed := stringSet("nodeId", "attempt", "outputPortIds", "outcomeSeqs", "deliveredEdges")
		value, decodeErr := decodeClosedProjection[SafeRetryTarget](item, allowed, allowed)
		if decodeErr != nil || !safeProjectionIdentifier(value.NodeID) || !projectionSafeSequence(value.Attempt) || value.OutputPortIDs == nil || value.OutcomeSeqs == nil || value.DeliveredEdges == nil || len(value.DeliveredEdges) != 0 || len(value.OutputPortIDs) != len(value.OutcomeSeqs) {
			return SafeRetryTarget{}, errors.New("invalid retry target")
		}
		seen := map[string]bool{}
		for _, portID := range value.OutputPortIDs {
			if !safeProjectionIdentifier(portID) || seen[portID] {
				return SafeRetryTarget{}, errors.New("invalid retry target port")
			}
			seen[portID] = true
		}
		if !projectionAscendingUnique(value.OutcomeSeqs, len(value.OutcomeSeqs) == 0) {
			return SafeRetryTarget{}, errors.New("invalid retry outcome")
		}
		return value, nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func decodeArtifactProjectionArray(raw json.RawMessage) ([]ArtifactProjection, error) {
	return closedProjectionArray(raw, decodeArtifactProjection)
}

func decodeDisplayEvidence(raw json.RawMessage) ([]SafeDisplayEvidence, error) {
	return closedProjectionArray(raw, func(item json.RawMessage) (SafeDisplayEvidence, error) {
		object, err := decodeUniqueJSONObject(item)
		if err != nil {
			return SafeDisplayEvidence{}, err
		}
		kind, err := requiredJSONString(object, "kind")
		if err != nil {
			return SafeDisplayEvidence{}, err
		}
		switch kind {
		case "text":
			value, err := decodeClosedProjection[SafeDisplayEvidence](item, stringSet("kind", "text"), stringSet("kind", "text"))
			if err != nil || value.Text == "" || len(value.Text) > 64<<10 {
				return SafeDisplayEvidence{}, errors.New("invalid text display evidence")
			}
			return value, nil
		case "artifact":
			for key := range object {
				if !stringSet("kind", "artifactProjection")[key] {
					return SafeDisplayEvidence{}, errors.New("invalid artifact display evidence")
				}
			}
			projection, err := decodeArtifactProjection(object["artifactProjection"])
			if err != nil {
				return SafeDisplayEvidence{}, err
			}
			return SafeDisplayEvidence{Kind: kind, ArtifactProjection: projection}, nil
		default:
			return SafeDisplayEvidence{}, errors.New("invalid display evidence kind")
		}
	})
}

func decodeArtifactSource(raw json.RawMessage) (SafeArtifactSource, error) {
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return SafeArtifactSource{}, err
	}
	kind, err := requiredJSONString(object, "kind")
	if err != nil {
		return SafeArtifactSource{}, err
	}
	var allowed map[string]bool
	switch kind {
	case "slot":
		allowed = stringSet("kind", "dispatchId", "nodeId", "slotId")
	case "gate":
		allowed = stringSet("kind", "gateId", "gateAttempt")
	case "system":
		allowed = stringSet("kind", "sourceId")
	default:
		return SafeArtifactSource{}, errors.New("invalid artifact source")
	}
	value, err := decodeClosedProjection[SafeArtifactSource](raw, allowed, allowed)
	if err != nil {
		return SafeArtifactSource{}, err
	}
	for _, identity := range []string{value.DispatchID, value.NodeID, value.SlotID, value.GateID, value.SourceID} {
		if identity != "" && !safeProjectionIdentifier(identity) {
			return SafeArtifactSource{}, errors.New("invalid artifact source identity")
		}
	}
	if kind == "gate" && !projectionSafeSequence(value.GateAttempt) {
		return SafeArtifactSource{}, errors.New("invalid artifact gate attempt")
	}
	return value, nil
}

func decodeNodeAttemptSnapshots(raw json.RawMessage) ([]SafeNodeAttemptSnapshot, error) {
	return closedProjectionArray(raw, func(item json.RawMessage) (SafeNodeAttemptSnapshot, error) {
		allowed := stringSet("nodeId", "nodeKind", "attempt", "startSeq", "phase", "phaseSeq")
		value, err := decodeClosedProjection[SafeNodeAttemptSnapshot](item, allowed, allowed)
		if err != nil || !safeProjectionIdentifier(value.NodeID) || !projectionValueIn(value.NodeKind, "mission", "formation", "tool", "gate") || !projectionSafeSequence(value.Attempt) || !projectionSafeSequence(value.StartSeq) || !projectionValueIn(value.Phase, "started", "gate_evaluating", "waiting_human") || !projectionSafeSequence(value.PhaseSeq) || value.PhaseSeq < value.StartSeq {
			return SafeNodeAttemptSnapshot{}, errors.New("invalid node attempt snapshot")
		}
		return value, nil
	})
}

func decodeToolLeaseSnapshots(raw json.RawMessage) ([]SafeToolLeaseSnapshot, error) {
	return closedProjectionArray(raw, func(item json.RawMessage) (SafeToolLeaseSnapshot, error) {
		allowed := stringSet("toolLeaseId", "nodeId", "attempt", "dispatchSeq", "latestLaunch")
		value, err := decodeClosedProjection[SafeToolLeaseSnapshot](item, allowed, stringSet("toolLeaseId", "nodeId", "attempt", "dispatchSeq"))
		if err != nil || !safeProjectionIdentifier(value.ToolLeaseID) || !safeProjectionIdentifier(value.NodeID) || !projectionSafeSequence(value.Attempt) || !projectionSafeSequence(value.DispatchSeq) {
			return SafeToolLeaseSnapshot{}, errors.New("invalid tool lease snapshot")
		}
		if value.LatestLaunch != nil {
			launchRaw := rawObjectMember(item, "latestLaunch")
			launchAllowed := stringSet("launchId", "generation", "processScopeId", "deadlineAuthorityId", "launchSeq")
			launch, err := decodeClosedProjection[SafeToolLaunchSnapshot](launchRaw, launchAllowed, launchAllowed)
			if err != nil || !safeProjectionIdentifier(launch.LaunchID) || !canonicalUnsignedDecimal(launch.Generation) || !safeProjectionIdentifier(launch.ProcessScopeID) || !safeProjectionIdentifier(launch.DeadlineAuthorityID) || !projectionSafeSequence(launch.LaunchSeq) {
				return SafeToolLeaseSnapshot{}, errors.New("invalid tool launch snapshot")
			}
			value.LatestLaunch = &launch
		}
		return value, nil
	})
}

func decodeFailureCause(raw json.RawMessage) (SafeFailureCause, error) {
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return SafeFailureCause{}, err
	}
	kind, err := requiredJSONString(object, "kind")
	if err != nil {
		return SafeFailureCause{}, err
	}
	allowed := stringSet("kind")
	switch kind {
	case "slot":
		allowed["dispatchId"] = true
	case "tool":
		allowed["toolLeaseId"] = true
	case "error":
		allowed["errorSeq"] = true
	case "none":
	default:
		return SafeFailureCause{}, errors.New("invalid failure cause")
	}
	value, err := decodeClosedProjection[SafeFailureCause](raw, allowed, allowed)
	if err != nil || (value.DispatchID != "" && !safeProjectionIdentifier(value.DispatchID)) || (value.ToolLeaseID != "" && !safeProjectionIdentifier(value.ToolLeaseID)) || (kind == "error" && !projectionSafeSequence(value.ErrorSeq)) {
		return SafeFailureCause{}, errors.New("invalid failure cause")
	}
	return value, nil
}

func schema2OpenDispatchMembers() map[string]bool {
	return stringSet("dispatchId", "targetLeaseId", "nodeId", "attempt", "slotId", "agentId", "bindingId", "sessionTargetId", "targetFingerprint", "dispatchSeq", "peekCapabilityState", "latestCapabilityGeneration", "latestCapabilityIssuedSeq", "latestSteeringGeneration", "openSteeringStartedSeq", "peekCapabilityRevokedSeq", "interruptState", "interruptRequestedSeq", "interruptOutcomeSeq")
}

func decodeNodeAttemptDispositions(raw json.RawMessage, allowedDispositions ...string) ([]SafeNodeAttemptDisposition, error) {
	return closedProjectionArray(raw, func(item json.RawMessage) (SafeNodeAttemptDisposition, error) {
		allowed := stringSet("nodeId", "nodeKind", "attempt", "startSeq", "phase", "phaseSeq", "disposition")
		value, err := decodeClosedProjection[SafeNodeAttemptDisposition](item, allowed, allowed)
		if err != nil || !projectionValueIn(value.Disposition, allowedDispositions...) {
			return SafeNodeAttemptDisposition{}, errors.New("invalid node attempt disposition")
		}
		snapshotRaw, _ := json.Marshal(value.SafeNodeAttemptSnapshot)
		snapshotsRaw, _ := json.Marshal([]json.RawMessage{snapshotRaw})
		snapshots, err := decodeNodeAttemptSnapshots(snapshotsRaw)
		if err != nil {
			return SafeNodeAttemptDisposition{}, err
		}
		value.SafeNodeAttemptSnapshot = snapshots[0]
		return value, nil
	})
}

func decodeSlotDispatchDispositions(raw json.RawMessage, allowedDispositions ...string) ([]SafeSlotDispatchDisposition, error) {
	return closedProjectionArray(raw, func(item json.RawMessage) (SafeSlotDispatchDisposition, error) {
		object, err := decodeUniqueJSONObject(item)
		if err != nil {
			return SafeSlotDispatchDisposition{}, err
		}
		baseAllowed := schema2OpenDispatchMembers()
		allowed := schema2OpenDispatchMembers()
		for _, key := range []string{"disposition", "softInterrupt", "softInterruptRequestedSeq", "softInterruptOutcomeSeq", "targetLeaseState", "finalPeekCapabilityState", "finalCapabilityGeneration", "finalCapabilityIssuedSeq", "finalSteeringGeneration", "finalPeekCapabilityRevokedSeq"} {
			allowed[key] = true
		}
		for key := range object {
			if !allowed[key] {
				return SafeSlotDispatchDisposition{}, errors.New("unknown slot disposition member")
			}
		}
		baseObject := map[string]json.RawMessage{}
		for key := range baseAllowed {
			if value, ok := object[key]; ok {
				baseObject[key] = value
			}
		}
		baseRaw, _ := json.Marshal([]map[string]json.RawMessage{baseObject})
		bases, err := decodeSchema2OpenDispatches(baseRaw)
		if err != nil || len(bases) != 1 {
			return SafeSlotDispatchDisposition{}, errors.New("invalid slot disposition dispatch")
		}
		clean, _ := json.Marshal(object)
		var value SafeSlotDispatchDisposition
		decoder := json.NewDecoder(bytes.NewReader(clean))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil || !projectionValueIn(value.Disposition, allowedDispositions...) || !projectionValueIn(value.SoftInterrupt, "sent", "send_uncertain", "unavailable", "unsupported") || !projectionSafeSequence(value.SoftInterruptRequestedSeq) || !projectionValueIn(value.TargetLeaseState, "released_quiescent", "terminal_hold", "quarantined") || value.FinalPeekCapabilityState != "revoked" || !canonicalUnsignedDecimal(value.FinalCapabilityGeneration) || !canonicalUnsignedDecimal(value.FinalSteeringGeneration) || !projectionSafeSequence(value.FinalPeekCapabilityRevokedSeq) {
			return SafeSlotDispatchDisposition{}, errors.New("invalid slot disposition")
		}
		if value.SoftInterrupt == "send_uncertain" {
			if value.SoftInterruptOutcomeSeq != 0 {
				return SafeSlotDispatchDisposition{}, errors.New("invalid uncertain interrupt")
			}
		} else if !projectionSafeSequence(value.SoftInterruptOutcomeSeq) {
			return SafeSlotDispatchDisposition{}, errors.New("missing interrupt outcome")
		}
		value.SafeSchema2OpenDispatch = bases[0]
		return value, nil
	})
}

func decodeToolLeaseDispositions(raw json.RawMessage, allowedDispositions ...string) ([]SafeToolLeaseDisposition, error) {
	return closedProjectionArray(raw, func(item json.RawMessage) (SafeToolLeaseDisposition, error) {
		allowed := stringSet("toolLeaseId", "nodeId", "attempt", "dispatchSeq", "latestLaunch", "disposition")
		value, err := decodeClosedProjection[SafeToolLeaseDisposition](item, allowed, stringSet("toolLeaseId", "nodeId", "attempt", "dispatchSeq", "disposition"))
		if err != nil || !projectionValueIn(value.Disposition, allowedDispositions...) {
			return SafeToolLeaseDisposition{}, errors.New("invalid tool lease disposition")
		}
		snapshotObject, _ := decodeUniqueJSONObject(item)
		delete(snapshotObject, "disposition")
		snapshotRaw, _ := json.Marshal([]map[string]json.RawMessage{snapshotObject})
		snapshots, err := decodeToolLeaseSnapshots(snapshotRaw)
		if err != nil || len(snapshots) != 1 {
			return SafeToolLeaseDisposition{}, errors.New("invalid tool lease disposition snapshot")
		}
		value.SafeToolLeaseSnapshot = snapshots[0]
		return value, nil
	})
}

func selectedProjectionDataHash(data map[string]json.RawMessage, keys ...string) (string, error) {
	selected := make(map[string]json.RawMessage, len(keys))
	for _, key := range keys {
		value, ok := data[key]
		if !ok {
			return "", errors.New("missing hashed member")
		}
		selected[key] = value
	}
	raw, err := json.Marshal(selected)
	if err != nil {
		return "", err
	}
	return projectionSHA(raw), nil
}

func validateAndNormalizeSchema2Data(eventType string, data map[string]json.RawMessage) error {
	switch eventType {
	case "run_resumed", "run_blocked":
		retryTargets, err := decodeRetryTargets(data["retryTargets"])
		if err != nil {
			return err
		}
		return replaceProjectionData(data, "retryTargets", retryTargets)
	case "slot_dispatch":
		turnInputs, err := decodeTurnInputs(data["turnInputs"])
		if err != nil {
			return err
		}
		if err := replaceProjectionData(data, "turnInputs", turnInputs); err != nil {
			return err
		}
		decoded, err := decodeTypedData[SafeSchema2SlotDispatchData](data)
		if err != nil || !safeProjectionIdentifier(decoded.DispatchID) || !safeProjectionIdentifier(decoded.TargetLeaseID) || !safeProjectionIdentifier(decoded.NodeID) || !safeProjectionIdentifier(decoded.SlotID) || !safeProjectionIdentifier(decoded.AgentID) || !safeProjectionIdentifier(decoded.BindingID) || !safeProjectionIdentifier(decoded.SessionTargetID) || !projectionSafeSequence(decoded.Attempt) || !lowercaseProjectionSHA(decoded.TargetFingerprint) || !lowercaseProjectionSHA(decoded.DispatchInputBarrierSHA256) || !lowercaseProjectionSHA(decoded.TargetReadyProofSHA256) || !lowercaseProjectionSHA(decoded.PaneHistoryBaselineSHA256) || !lowercaseProjectionSHA(decoded.PromptSHA256) || !canonicalUnsignedDecimal(decoded.SteeringGeneration) || !decoded.RecordedBeforeSend {
			return errors.New("invalid slot dispatch")
		}
	case "slot_result":
		allowed := stringSet("turnKey", "phase", "status", "turnPayload", "outputs", "reportArtifactId", "artifactIds", "diffArtifactIds")
		turnResult, err := decodeClosedProjection[SafeTurnResult](data["turnResult"], allowed, allowed)
		if err != nil || !projectionValueIn(turnResult.Status, "done", "needs-review", "failed") {
			return errors.New("invalid turn result")
		}
		outer, err := decodeTypedData[SafeSchema2SlotResultData](data)
		if err != nil || outer.TurnKey != turnResult.TurnKey || outer.TurnPhase != turnResult.Phase || !projectionValueIn(outer.Status, "ok", "error", "timeout") {
			return errors.New("turn result identity mismatch")
		}
		turnPayload, err := decodePayloadProjection(rawObjectMember(data["turnResult"], "turnPayload"))
		if err != nil {
			return err
		}
		outputs, err := decodePayloadProjections(rawObjectMember(data["turnResult"], "outputs"))
		if err != nil {
			return err
		}
		turnResult.TurnPayload = turnPayload
		turnResult.Outputs = outputs
		canonical, err := canonicalProjectionJSON(data["turnResult"])
		if err != nil || projectionSHA(canonical) != outer.TurnResultSHA256 || outer.TurnResultEncoding != "slot-turn-result-jcs-v1" {
			return errors.New("turn result hash mismatch")
		}
		return replaceProjectionData(data, "turnResult", turnResult)
	case "formation_result":
		decoded, err := decodeTypedData[SafeSchema2FormationResultData](data)
		if err != nil || !projectionValueIn(decoded.Status, "done", "needs-review", "failed") || !safeProjectionIdentifier(decoded.NodeID) || !projectionSafeSequence(decoded.Attempt) || decoded.ResultEncoding != "formation-result-jcs-v1" || !projectionAscendingUnique(decoded.ContributingSlotResultSeqs, false) {
			return errors.New("invalid formation result")
		}
		outputs, err := decodePayloadProjections(data["outputs"])
		if err != nil {
			return err
		}
		hashes, err := decodeProjectionHashes(data["outputHashes"])
		if err != nil || validateRawProjectionHashes(data["outputs"], hashes) != nil {
			return errors.New("invalid formation output hashes")
		}
		hash, err := selectedProjectionDataHash(data, "status", "outputs", "outputHashes", "reportArtifactId", "artifactIds", "diffArtifactIds", "contributingSlotResultSeqs")
		if err != nil || hash != decoded.ResultSHA256 {
			return errors.New("formation result hash mismatch")
		}
		if err := replaceProjectionData(data, "outputs", outputs); err != nil {
			return err
		}
		return replaceProjectionData(data, "outputHashes", hashes)
	case "tool_dispatch":
		decoded, err := decodeTypedData[SafeSchema2ToolDispatchData](data)
		if err != nil || !safeProjectionIdentifier(decoded.ToolLeaseID) || !safeProjectionIdentifier(decoded.NodeID) || !projectionSafeSequence(decoded.Attempt) || !safeProjectionIdentifier(decoded.ToolBindingID) || !lowercaseProjectionSHA(decoded.InputManifestSHA256) || !lowercaseProjectionSHA(decoded.ProfileSHA256) || !lowercaseProjectionSHA(decoded.ParametersSHA256) || !lowercaseProjectionSHA(decoded.PolicySHA256) || !lowercaseProjectionSHA(decoded.DeterminismPolicySHA256) || !lowercaseProjectionSHA(decoded.ExecutionBundleSHA256) || !decoded.RecordedBeforeExecute {
			return errors.New("invalid tool dispatch")
		}
		hashes, err := decodeProjectionHashes(data["inputHashes"])
		if err != nil {
			return err
		}
		return replaceProjectionData(data, "inputHashes", hashes)
	case "tool_result":
		decoded, err := decodeTypedData[SafeSchema2ToolResultData](data)
		if err != nil || !projectionValueIn(decoded.Status, "ok", "error", "timeout") || !safeProjectionIdentifier(decoded.ToolLeaseID) || !safeProjectionIdentifier(decoded.LaunchID) || !canonicalUnsignedDecimal(decoded.Generation) || !safeProjectionIdentifier(decoded.NodeID) || !projectionSafeSequence(decoded.Attempt) {
			return errors.New("invalid tool result")
		}
		outputs, err := decodePayloadProjections(data["outputs"])
		if err != nil {
			return err
		}
		hashes, err := decodeProjectionHashes(data["outputHashes"])
		if err != nil || validateRawProjectionHashes(data["outputs"], hashes) != nil {
			return errors.New("invalid tool output hashes")
		}
		registrations, err := decodeArtifactProjectionArray(data["artifactRegistrations"])
		if err != nil {
			return err
		}
		seenArtifacts := map[string]bool{}
		for _, projection := range registrations {
			artifactID := artifactProjectionID(projection)
			if seenArtifacts[artifactID] {
				return errors.New("duplicate artifact registration")
			}
			seenArtifacts[artifactID] = true
		}
		artifacts, err := decodeArtifactProjectionArray(data["artifacts"])
		if err != nil {
			return err
		}
		display, err := decodeDisplayEvidence(data["displayEvidence"])
		if err != nil {
			return err
		}
		timing, err := decodeEventTiming(data["timing"])
		if err != nil {
			return err
		}
		for key, value := range map[string]any{"outputs": outputs, "outputHashes": hashes, "artifactRegistrations": registrations, "artifacts": artifacts, "displayEvidence": display, "timing": timing} {
			if err := replaceProjectionData(data, key, value); err != nil {
				return err
			}
		}
	case "node_output":
		decoded, err := decodeTypedData[SafeSchema2NodeOutputData](data)
		if err != nil || !safeProjectionIdentifier(decoded.NodeID) || !projectionValueIn(decoded.Status, "done", "needs-review", "blocked", "failed") {
			return errors.New("invalid node output")
		}
		outputs, err := decodePayloadProjections(data["outputs"])
		if err != nil {
			return err
		}
		producerAllowed := stringSet("kind", "outcomeSeq")
		producer, err := decodeClosedProjection[SafeProducedBy](data["producedBy"], producerAllowed, producerAllowed)
		if err != nil || !projectionValueIn(producer.Kind, "mission", "formation", "tool", "gate") || !projectionSafeSequence(producer.OutcomeSeq) {
			return errors.New("invalid output producer")
		}
		timing, err := decodeEventTiming(data["timing"])
		if err != nil {
			return err
		}
		edges, err := closedProjectionArray(data["deliveredEdges"], func(item json.RawMessage) (SafeDeliveredEdge, error) {
			allowed := stringSet("originEdgeId", "deliveryEdgeId", "toNodeId", "toPortId", "sourceNodeId", "sourcePortId", "sourceOutputSeq", "sourceAttempt")
			value, decodeErr := decodeClosedProjection[SafeDeliveredEdge](item, allowed, allowed)
			if decodeErr != nil || !safeProjectionIdentifier(value.OriginEdgeID) || !safeProjectionIdentifier(value.DeliveryEdgeID) || !safeProjectionIdentifier(value.ToNodeID) || !safeProjectionIdentifier(value.ToPortID) || !safeProjectionIdentifier(value.SourceNodeID) || !safeProjectionIdentifier(value.SourcePortID) || !projectionSafeSequence(value.SourceOutputSeq) || !projectionSafeSequence(value.SourceAttempt) {
				return SafeDeliveredEdge{}, errors.New("invalid delivered edge")
			}
			return value, nil
		})
		if err != nil {
			return err
		}
		for key, value := range map[string]any{"outputs": outputs, "producedBy": producer, "timing": timing, "deliveredEdges": edges} {
			if err := replaceProjectionData(data, key, value); err != nil {
				return err
			}
		}
	case "gate_evaluating":
		allowed := stringSet("classification", "sourceKind", "encoding", "mediaType", "sha256", "text")
		criterion, err := decodeClosedProjection[SafeCriterionProjection](data["criterionProjection"], allowed, allowed)
		if err != nil || criterion.Classification != "authored_config" || criterion.SourceKind != "gate_criterion" || criterion.Encoding != "gate-criterion-utf8-v1" || criterion.MediaType != "text/plain" || criterion.Text == "" || len(criterion.Text) > 64<<10 || criterion.SHA256 != projectionSHA([]byte(criterion.Text)) {
			return errors.New("invalid gate criterion")
		}
		return replaceProjectionData(data, "criterionProjection", criterion)
	case "gate_kind_result":
		decoded, err := decodeTypedData[SafeSchema2GateKindResultData](data)
		if err != nil || !projectionValueIn(decoded.Kind, "code", "formation") || !projectionValueIn(decoded.Verdict, "pass", "fail") || decoded.ResultEncoding != "gate-kind-result-jcs-v1" {
			return errors.New("invalid gate kind result")
		}
		if err := validateSafeGateEvidence(decoded.Evidence); err != nil {
			return err
		}
		if decoded.Kind == "code" {
			if !safeProjectionIdentifier(decoded.GateBindingID) || !lowercaseProjectionSHA(decoded.InputSHA256) || !lowercaseProjectionSHA(decoded.ProfileSHA256) || !lowercaseProjectionSHA(decoded.EvaluatorBundleSHA256) || !lowercaseProjectionSHA(decoded.ParametersSHA256) || !lowercaseProjectionSHA(decoded.PolicySHA256) || !lowercaseProjectionSHA(decoded.DeterminismPolicySHA256) || len(decoded.RelatedSeqs) != 0 {
				return errors.New("invalid code gate binding")
			}
		} else if decoded.GateBindingID != "" || decoded.InputSHA256 != "" || decoded.ProfileSHA256 != "" || decoded.EvaluatorBundleSHA256 != "" || decoded.ParametersSHA256 != "" || decoded.PolicySHA256 != "" || decoded.DeterminismPolicySHA256 != "" || !projectionAscendingUnique(decoded.RelatedSeqs, false) {
			return errors.New("invalid formation gate binding")
		}
		hash, err := selectedProjectionDataHash(data, "verdict", "reason", "evidence")
		if err != nil || hash != decoded.ResultSHA256 {
			return errors.New("gate kind result hash mismatch")
		}
	case "judge_result":
		decoded, err := decodeTypedData[SafeSchema2JudgeResultData](data)
		if err != nil || decoded.ResultEncoding != "judge-result-jcs-v1" || !projectionValueIn(decoded.Result.Verdict, "pass", "fail") || decoded.Result.Reason == "" || validateSafeGateEvidence(decoded.Result.Evidence) != nil {
			return errors.New("invalid judge result")
		}
		canonical, err := canonicalProjectionJSON(data["result"])
		if err != nil || projectionSHA(canonical) != decoded.ResultSHA256 {
			return errors.New("judge result hash mismatch")
		}
	case "gate_verdict":
		decoded, err := decodeTypedData[SafeSchema2GateVerdictData](data)
		if err != nil || !projectionValueIn(decoded.Verdict, "pass", "fail") || len(decoded.PerKind) == 0 || len(decoded.PerKind) != len(decoded.KindResultSeqs) {
			return errors.New("invalid gate verdict")
		}
		for kind, verdict := range decoded.PerKind {
			if !safeProjectionIdentifier(kind) || !projectionValueIn(verdict, "pass", "fail", "not_run") {
				return errors.New("invalid gate kind verdict")
			}
			sequence, ok := decoded.KindResultSeqs[kind]
			if !ok || (verdict == "not_run" && sequence != nil) || (verdict != "not_run" && (sequence == nil || !projectionSafeSequence(*sequence))) {
				return errors.New("invalid gate result sequence")
			}
		}
		feedback, err := decodeSafeGateFeedback(data["feedbackPayload"])
		if err != nil {
			return err
		}
		return replaceProjectionData(data, "feedbackPayload", feedback)
	case "human_input_requested":
		promptAllowed := stringSet("classification", "sourceKind", "templateId")
		prompt, err := decodeClosedProjection[SafeFixedSystemProjection](data["promptProjection"], promptAllowed, promptAllowed)
		if err != nil || prompt.Classification != "fixed_system" || prompt.SourceKind != "human_prompt" || prompt.TemplateID != "gate-human-verdict-v1" {
			return errors.New("invalid human prompt projection")
		}
		choices, err := decodeClosedProjection[SafeHumanChoiceProjections](data["choiceProjections"], stringSet("pass", "fail"), stringSet("pass", "fail"))
		if err != nil || choices.Pass.Classification != "fixed_system" || choices.Pass.SourceKind != "human_choice" || choices.Pass.TemplateID != "gate-human-pass-v1" || choices.Fail.Classification != "fixed_system" || choices.Fail.SourceKind != "human_choice" || choices.Fail.TemplateID != "gate-human-fail-v1" {
			return errors.New("invalid human choice projections")
		}
		var completed SafeCompletedKindResultSeqs
		if err := json.Unmarshal(data["completedKindResultSeqs"], &completed); err != nil || completed == nil {
			return errors.New("invalid completed kind results")
		}
		for kind, sequence := range completed {
			if !safeProjectionIdentifier(kind) || !projectionSafeSequence(sequence) {
				return errors.New("invalid completed kind result")
			}
		}
		for key, value := range map[string]any{"promptProjection": prompt, "choiceProjections": choices, "completedKindResultSeqs": completed} {
			if err := replaceProjectionData(data, key, value); err != nil {
				return err
			}
		}
	case "run_cancel_requested":
		nodes, err := decodeNodeAttemptSnapshots(data["openNodeAttempts"])
		if err != nil {
			return err
		}
		slots, err := decodeSchema2OpenDispatches(data["openSlotDispatches"])
		if err != nil {
			return err
		}
		tools, err := decodeToolLeaseSnapshots(data["openToolLeases"])
		if err != nil {
			return err
		}
		for key, value := range map[string]any{"openNodeAttempts": nodes, "openSlotDispatches": slots, "openToolLeases": tools} {
			if err := replaceProjectionData(data, key, value); err != nil {
				return err
			}
		}
	case "run_failure_reconciliation_started":
		cause, err := decodeFailureCause(data["failureCause"])
		if err != nil {
			return err
		}
		nodes, err := decodeNodeAttemptSnapshots(data["openNodeAttempts"])
		if err != nil {
			return err
		}
		slots, err := decodeSchema2OpenDispatches(data["openSlotDispatches"])
		if err != nil {
			return err
		}
		tools, err := decodeToolLeaseSnapshots(data["openToolLeases"])
		if err != nil {
			return err
		}
		var recorded bool
		if json.Unmarshal(data["recordedBeforeReconciliation"], &recorded) != nil || !recorded {
			return errors.New("failure reconciliation not recorded")
		}
		for key, value := range map[string]any{"failureCause": cause, "openNodeAttempts": nodes, "openSlotDispatches": slots, "openToolLeases": tools} {
			if err := replaceProjectionData(data, key, value); err != nil {
				return err
			}
		}
	case "run_canceled":
		var final bool
		if json.Unmarshal(data["final"], &final) != nil || !final {
			return errors.New("run canceled is not final")
		}
		nodes, err := decodeNodeAttemptDispositions(data["nodeAttemptDispositions"], "canceled_non_authorizing")
		if err != nil {
			return err
		}
		slots, err := decodeSlotDispatchDispositions(data["slotDispatchDispositions"], "canceled_non_authorizing")
		if err != nil {
			return err
		}
		tools, err := decodeToolLeaseDispositions(data["reconciledToolLeases"], "never_launched_cleaned", "launch_fenced_cleaned")
		if err != nil {
			return err
		}
		for _, tool := range tools {
			if (tool.LatestLaunch == nil) != (tool.Disposition == "never_launched_cleaned") {
				return errors.New("tool cancellation disposition mismatch")
			}
		}
		for key, value := range map[string]any{"nodeAttemptDispositions": nodes, "slotDispatchDispositions": slots, "reconciledToolLeases": tools} {
			if err := replaceProjectionData(data, key, value); err != nil {
				return err
			}
		}
	case "run_failed":
		var final bool
		if json.Unmarshal(data["final"], &final) != nil || !final {
			return errors.New("run failed is not final")
		}
		cause, err := decodeFailureCause(data["failureCause"])
		if err != nil {
			return err
		}
		nodes, err := decodeNodeAttemptDispositions(data["nodeAttemptDispositions"], "failed_non_authorizing", "abandoned_non_authorizing")
		if err != nil {
			return err
		}
		slots, err := decodeSlotDispatchDispositions(data["slotDispatchDispositions"], "failed_non_authorizing", "abandoned_non_authorizing")
		if err != nil {
			return err
		}
		tools, err := decodeToolLeaseDispositions(data["toolLeaseDispositions"], "failed_private_cleanup_owned", "abandoned_private_cleanup_owned")
		if err != nil {
			return err
		}
		for key, value := range map[string]any{"failureCause": cause, "nodeAttemptDispositions": nodes, "slotDispatchDispositions": slots, "toolLeaseDispositions": tools} {
			if err := replaceProjectionData(data, key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

func sanitizeSchema2Event(event rawProjectionEvent) (SafeRunEvent, error) {
	allowed, private := schema2DataContract(event.typeName)
	if allowed == nil {
		return nil, projectionError(ErrRunEventUnknown, "unknown schema-2 event type")
	}
	data, err := validateDataKeys(event.data, allowed, private)
	if err != nil {
		return nil, err
	}
	envelope := eventEnvelope(event)
	if raw, ok := data["inputRef"]; ok {
		clean, e := sanitizeInputIdentity(raw)
		if e != nil {
			return nil, e
		}
		data["inputRef"] = clean
	}
	if raw, ok := data["evaluatedInputRef"]; ok {
		clean, e := sanitizeInputIdentity(raw)
		if e != nil {
			return nil, e
		}
		data["evaluatedInputRef"] = clean
	}
	if raw, ok := data["inputRefs"]; ok {
		clean, e := sanitizeInputIdentityArray(raw)
		if e != nil {
			return nil, e
		}
		data["inputRefs"] = clean
	}
	if err := validateAndNormalizeSchema2Data(event.typeName, data); err != nil {
		return nil, projectionError(ErrRunEventUnknown, "invalid schema-2 nested projection")
	}
	makeError := func() error { return projectionError(ErrRunEventUnknown, "invalid schema-2 event data") }
	switch event.typeName {
	case "run_started":
		decoded, e := decodeTypedData[SafeSchema2RunStartedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2RunStartedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "run_activated":
		decoded, e := decodeTypedData[SafeSchema2RunActivatedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2RunActivatedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "run_resumed":
		decoded, e := decodeSchema2RunResumedData(data)
		if e != nil {
			return nil, e
		}
		return SafeSchema2RunResumedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "node_waiting":
		decoded, e := decodeTypedData[SafeSchema2NodeWaitingData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2NodeWaitingEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "node_input_ignored":
		decoded, e := decodeTypedData[SafeSchema2NodeInputIgnoredData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2NodeInputIgnoredEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "node_started":
		decoded, e := decodeTypedData[SafeSchema2NodeStartedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2NodeStartedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "slot_binding_observed":
		decoded, e := decodeTypedData[SafeSchema2SlotBindingObservedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2SlotBindingObservedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "slot_dispatch":
		decoded, e := decodeTypedData[SafeSchema2SlotDispatchData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2SlotDispatchEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "slot_peek_capability_issued":
		decoded, e := decodeTypedData[SafeSchema2SlotPeekCapabilityIssuedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2SlotPeekCapabilityIssuedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "slot_steering_started":
		decoded, e := decodeTypedData[SafeSchema2SlotSteeringStartedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2SlotSteeringStartedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "slot_steering_ended":
		decoded, e := decodeTypedData[SafeSchema2SlotSteeringEndedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2SlotSteeringEndedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "slot_peek_capability_revoked":
		decoded, e := decodeTypedData[SafeSchema2SlotPeekCapabilityRevokedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2SlotPeekCapabilityRevokedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "slot_reconciliation_interrupt":
		decoded, e := decodeTypedData[SafeSchema2SlotReconciliationInterruptData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2SlotReconciliationInterruptEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "slot_reconciliation_interrupt_outcome":
		decoded, e := decodeTypedData[SafeSchema2SlotReconciliationInterruptOutcomeData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2SlotReconciliationInterruptOutcomeEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "slot_result":
		decoded, e := decodeTypedData[SafeSchema2SlotResultData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2SlotResultEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "formation_result":
		decoded, e := decodeTypedData[SafeSchema2FormationResultData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2FormationResultEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "tool_dispatch":
		decoded, e := decodeTypedData[SafeSchema2ToolDispatchData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2ToolDispatchEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "tool_process_launch":
		decoded, e := decodeTypedData[SafeSchema2ToolProcessLaunchData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2ToolProcessLaunchEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "tool_result":
		decoded, e := decodeTypedData[SafeSchema2ToolResultData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2ToolResultEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "node_output":
		decoded, e := decodeTypedData[SafeSchema2NodeOutputData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2NodeOutputEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "gate_evaluating":
		decoded, e := decodeTypedData[SafeSchema2GateEvaluatingData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2GateEvaluatingEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "gate_kind_result":
		decoded, e := decodeTypedData[SafeSchema2GateKindResultData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2GateKindResultEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "judge_result":
		decoded, e := decodeTypedData[SafeSchema2JudgeResultData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2JudgeResultEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "judge_attempt_failed":
		decoded, e := decodeTypedData[SafeSchema2JudgeAttemptFailedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2JudgeAttemptFailedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "gate_verdict":
		decoded, e := decodeTypedData[SafeSchema2GateVerdictData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2GateVerdictEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "artifact_attached":
		projection, e := decodeArtifactProjection(data["artifactProjection"])
		if e != nil {
			return nil, e
		}
		source, e := decodeArtifactSource(data["source"])
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2ArtifactAttachedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: SafeSchema2ArtifactAttachedData{ArtifactProjection: projection, Source: source}}, nil
	case "artifact_observed":
		decoded, e := decodeTypedData[SafeSchema2ArtifactObservedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2ArtifactObservedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "escalation_raised":
		decoded, e := decodeTypedData[SafeSchema2EscalationRaisedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2EscalationRaisedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "human_input_requested":
		decoded, e := decodeTypedData[SafeSchema2HumanInputRequestedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2HumanInputRequestedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "human_verdict_recorded":
		decoded, e := decodeTypedData[SafeSchema2HumanVerdictRecordedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2HumanVerdictRecordedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "error":
		decoded, e := decodeTypedData[SafeSchema2ErrorData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2ErrorEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "run_blocked":
		decoded, e := decodeSchema2RunBlockedData(data)
		if e != nil {
			return nil, e
		}
		return SafeSchema2RunBlockedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "run_cancel_requested":
		decoded, e := decodeTypedData[SafeSchema2RunCancelRequestedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2RunCancelRequestedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "run_canceled":
		decoded, e := decodeTypedData[SafeSchema2RunCanceledData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2RunCanceledEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "run_failure_reconciliation_started":
		decoded, e := decodeTypedData[SafeSchema2RunFailureReconciliationStartedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2RunFailureReconciliationStartedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "run_failed":
		decoded, e := decodeTypedData[SafeSchema2RunFailedData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2RunFailedEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	case "run_succeeded":
		decoded, e := decodeTypedData[SafeSchema2RunSucceededData](data)
		if e != nil {
			return nil, makeError()
		}
		return SafeSchema2RunSucceededEvent{safeEventEnvelope: envelope, Type: event.typeName, Data: decoded}, nil
	default:
		return nil, projectionError(ErrRunEventUnknown, "unknown schema-2 event type")
	}
}

func schema2DataContract(eventType string) (map[string]bool, map[string]bool) {
	contracts := map[string][]string{
		"run_started":                           {"workspaceAuthorityId", "workspaceAdmissionSeq", "admissionPolicyRev", "admissionPolicySha256", "admissionCommandId", "commandPayloadSha256", "boardSlug", "sourceBoardSchema", "snapshotSchema", "runAuthorityId", "graphSnapshotSha256", "privateBindingsSha256", "bindingProjectionSha256", "runRoot", "rootInputProjection", "limits"},
		"run_activated":                         {"workspaceAdmissionSeq", "admissionPolicyRev", "admissionPolicySha256", "reason"},
		"run_resumed":                           {"commandId", "commandPayloadSha256", "resumedFromSeq", "resumedBy", "resumeMode", "reason", "openDispatches", "retryTargets"},
		"node_waiting":                          {"neededInputs", "readyInputs", "totalInputs", "waitingFor"},
		"node_input_ignored":                    {"nodeId", "toPortId", "inputRef", "reason", "relatedAttempt"},
		"node_started":                          {"nodeId", "nodeKind", "attempt", "reason", "inputRefs", "contextEncoding", "judgeContextSha256", "priorResultSeqs", "triggerFeedbackId", "priorGateSeq"},
		"slot_binding_observed":                 {"bindingId", "slotId", "sessionTargetId", "health", "reason", "observedAt", "relatedSeq"},
		"slot_dispatch":                         {"dispatchId", "targetLeaseId", "turnKey", "turnPhase", "turnInputs", "nodeId", "attempt", "slotId", "agentId", "harness", "bindingId", "sessionTargetId", "targetFingerprint", "dispatchInputBarrierEncoding", "dispatchInputBarrierSha256", "targetReadyProofEncoding", "targetReadyProofSha256", "paneHistoryBaselineEncoding", "paneHistoryBaselineSha256", "steeringGeneration", "promptSha256", "nativeAck", "recordedBeforeSend"},
		"slot_peek_capability_issued":           {"dispatchId", "targetLeaseId", "bindingId", "sessionTargetId", "targetFingerprint", "capabilityGeneration", "priorIssuedSeq", "issuedAt"},
		"slot_steering_started":                 {"dispatchId", "targetLeaseId", "bindingId", "sessionTargetId", "targetFingerprint", "capabilityIssuedSeq", "capabilityGeneration", "steeringGeneration", "actor", "startedAt", "recordedBeforeInput"},
		"slot_steering_ended":                   {"startedSeq", "dispatchId", "targetLeaseId", "targetFingerprint", "steeringGeneration", "reason", "endedAt"},
		"slot_peek_capability_revoked":          {"dispatchId", "targetLeaseId", "bindingId", "sessionTargetId", "targetFingerprint", "capabilityGeneration", "capabilityIssuedSeq", "steeringGeneration", "reason", "revokedAt", "inputClosed"},
		"slot_reconciliation_interrupt":         {"dispatchId", "targetLeaseId", "bindingId", "sessionTargetId", "targetFingerprint", "authorityKind", "authoritySeq", "interruptEncoding", "interruptSha256", "recordedBeforeSend"},
		"slot_reconciliation_interrupt_outcome": {"requestedSeq", "dispatchId", "targetLeaseId", "targetFingerprint", "outcome", "observedAt"},
		"slot_result":                           {"dispatchId", "targetLeaseId", "turnKey", "turnPhase", "nodeId", "attempt", "slotId", "agentId", "bindingId", "sessionTargetId", "targetFingerprint", "paneHistoryBaselineEncoding", "paneHistoryBaselineDispatchSeq", "paneHistoryBaselineSha256", "peekCapabilityRevokedSeq", "steeringGeneration", "operatorInfluenced", "status", "turnResult", "turnResultEncoding", "turnResultSha256", "clientAttachmentAuditProofSha256"},
		"formation_result":                      {"nodeId", "attempt", "status", "outputs", "outputHashes", "reportArtifactId", "artifactIds", "diffArtifactIds", "contributingSlotResultSeqs", "resultEncoding", "resultSha256"},
		"tool_dispatch":                         {"toolLeaseId", "nodeId", "attempt", "toolBindingId", "inputManifestSha256", "inputHashes", "profileSha256", "parametersSha256", "policySha256", "determinismPolicySha256", "executionBundleSha256", "recordedBeforeExecute"},
		"tool_process_launch":                   {"toolLeaseId", "launchId", "nodeId", "attempt", "generation", "recordedBeforeSpawn"},
		"tool_result":                           {"toolLeaseId", "launchId", "generation", "nodeId", "attempt", "status", "outputs", "outputHashes", "artifactRegistrations", "artifacts", "displayEvidence", "timing"},
		"node_output":                           {"nodeId", "status", "outputs", "reportArtifactId", "artifactIds", "diffArtifactIds", "producedBy", "timing", "deliveredEdges"},
		"gate_evaluating":                       {"gateId", "gateAttempt", "nodeId", "kinds", "criterionProjection", "inputRef", "judgeChain", "revisionCycleId", "triggerFeedbackId", "priorGateSeq"},
		"gate_kind_result":                      {"gateId", "gateAttempt", "kind", "verdict", "reason", "evidence", "evaluatedInputRef", "resultEncoding", "resultSha256", "relatedSeqs", "gateBindingId", "inputSha256", "profileSha256", "evaluatorBundleSha256", "parametersSha256", "policySha256", "determinismPolicySha256"},
		"judge_result":                          {"gateId", "gateAttempt", "judgeNodeId", "judgeAttempt", "chainIndex", "contextEncoding", "contextSha256", "priorResultSeqs", "result", "resultEncoding", "resultSha256"},
		"judge_attempt_failed":                  {"gateId", "gateAttempt", "judgeNodeId", "judgeAttempt", "chainIndex", "contextSha256", "priorResultSeqs", "code", "reason", "relatedSeq"},
		"gate_verdict":                          {"gateId", "gateAttempt", "verdict", "perKind", "kindResultSeqs", "evaluatedInputRef", "routePort", "routedEdges", "reason", "feedbackPayload"},
		"artifact_attached":                     {"artifactProjection", "source"},
		"artifact_observed":                     {"artifactId", "availability", "artifact", "errorCode", "observedAt", "relatedSeq"},
		"escalation_raised":                     {"trigger", "severity", "reason", "source", "nodeId", "gateId", "blocks"},
		"human_input_requested":                 {"gateId", "gateAttempt", "nodeId", "promptProjection", "choiceProjections", "requestedBy", "evaluatedInputRef", "completedKindResultSeqs"},
		"human_verdict_recorded":                {"commandId", "commandPayloadSha256", "gateId", "gateAttempt", "nodeId", "verdict", "reason", "requestedSeq", "decidedBy"},
		"error":                                 {"code", "message", "boundary", "errorScope", "nodeId", "gateId", "slotId", "toolLeaseId", "recoverable", "relatedSeq"},
		"run_blocked":                           {"reason", "blockScope", "blockedNodeId", "blockedGateId", "resumeAllowed", "resumePolicy", "openDispatches", "retryTargets", "nextEpoch"},
		"run_cancel_requested":                  {"commandId", "commandPayloadSha256", "reason", "requestedBy", "openNodeAttempts", "openSlotDispatches", "openToolLeases"},
		"run_canceled":                          {"cancelRequestSeq", "reason", "requestedBy", "nodeAttemptDispositions", "slotDispatchDispositions", "reconciledToolLeases", "final"},
		"run_failure_reconciliation_started":    {"originCancelRequestSeq", "code", "reason", "unrecoverable", "relatedSeq", "failureCause", "openNodeAttempts", "openSlotDispatches", "openToolLeases", "recordedBeforeReconciliation"},
		"run_failed":                            {"failureReconciliationSeq", "code", "reason", "unrecoverable", "relatedSeq", "failureCause", "nodeAttemptDispositions", "slotDispatchDispositions", "toolLeaseDispositions", "final"},
		"run_succeeded":                         {"summaryArtifactId", "outputArtifactIds", "final"},
	}
	keys, ok := contracts[eventType]
	if !ok {
		return nil, nil
	}
	private := map[string]bool{}
	switch eventType {
	case "run_started":
		private = stringSet("boardPath")
	case "slot_dispatch":
		private = stringSet("dispatchInputBarrier", "targetReadyProof", "paneHistoryBaseline")
	case "slot_result":
		private = stringSet("capturedRange", "sentinel", "clientAttachmentAuditProof", "turnClosureProof")
	}
	return stringSet(keys...), private
}

func decodeSchema2OpenDispatches(raw json.RawMessage) ([]SafeSchema2OpenDispatch, error) {
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) != nil {
		return nil, projectionError(ErrRunProjectionInvalid, "invalid schema-2 open dispatches")
	}
	result := make([]SafeSchema2OpenDispatch, len(items))
	seen := map[string]bool{}
	allowed := stringSet("dispatchId", "targetLeaseId", "nodeId", "attempt", "slotId", "agentId", "bindingId", "sessionTargetId", "targetFingerprint", "dispatchSeq", "peekCapabilityState", "latestCapabilityGeneration", "latestCapabilityIssuedSeq", "latestSteeringGeneration", "openSteeringStartedSeq", "peekCapabilityRevokedSeq", "interruptState", "interruptRequestedSeq", "interruptOutcomeSeq")
	for index, item := range items {
		object, err := decodeUniqueJSONObject(item)
		if err != nil {
			return nil, projectionError(ErrRunProjectionInvalid, "invalid schema-2 dispatch")
		}
		for key := range object {
			if !allowed[key] {
				return nil, projectionError(ErrRunProjectionInvalid, "unknown schema-2 dispatch member")
			}
		}
		clean, _ := json.Marshal(object)
		decoder := json.NewDecoder(bytes.NewReader(clean))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&result[index]) != nil {
			return nil, projectionError(ErrRunProjectionInvalid, "invalid schema-2 dispatch")
		}
		itemValue := &result[index]
		if !safeProjectionIdentifier(itemValue.DispatchID) || !safeProjectionIdentifier(itemValue.TargetLeaseID) || !safeProjectionIdentifier(itemValue.NodeID) || !safeProjectionIdentifier(itemValue.SlotID) || !safeProjectionIdentifier(itemValue.AgentID) || !safeProjectionIdentifier(itemValue.BindingID) || !safeProjectionIdentifier(itemValue.SessionTargetID) || itemValue.Attempt == 0 || itemValue.Attempt > MaxJSONSafeInteger || itemValue.DispatchSeq == 0 || itemValue.DispatchSeq > MaxJSONSafeInteger {
			return nil, projectionError(ErrRunProjectionInvalid, "invalid schema-2 dispatch identity")
		}
		if seen[itemValue.DispatchID] {
			return nil, projectionError(ErrRunProjectionInvalid, "duplicate schema-2 dispatch")
		}
		seen[itemValue.DispatchID] = true
		if !canonicalUnsignedDecimal(itemValue.LatestCapabilityGeneration) || !canonicalUnsignedDecimal(itemValue.LatestSteeringGeneration) {
			return nil, projectionError(ErrRunProjectionInvalid, "invalid dispatch generation")
		}
		if itemValue.PeekCapabilityState == "none" {
			if itemValue.LatestCapabilityGeneration != "0" || itemValue.LatestCapabilityIssuedSeq != 0 || itemValue.PeekCapabilityRevokedSeq != nil {
				return nil, projectionError(ErrRunProjectionInvalid, "invalid none capability state")
			}
		} else if itemValue.PeekCapabilityState == "revoked" {
			if itemValue.PeekCapabilityRevokedSeq == nil {
				return nil, projectionError(ErrRunProjectionInvalid, "missing revocation sequence")
			}
		} else {
			return nil, projectionError(ErrRunProjectionInvalid, "invalid capability state")
		}
		if itemValue.InterruptState == "none" {
			if itemValue.InterruptRequestedSeq != nil || itemValue.InterruptOutcomeSeq != nil {
				return nil, projectionError(ErrRunProjectionInvalid, "invalid interrupt state")
			}
		} else if itemValue.InterruptState == "" {
			return nil, projectionError(ErrRunProjectionInvalid, "missing interrupt state")
		}
	}
	return result, nil
}

func canonicalUnsignedDecimal(value string) bool {
	if value == "0" {
		return true
	}
	if value == "" || value[0] == '0' {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}

func decodeSchema2RunBlockedData(data map[string]json.RawMessage) (SafeSchema2RunBlockedData, error) {
	var result SafeSchema2RunBlockedData
	raw, ok := data["openDispatches"]
	if !ok {
		return result, projectionError(ErrRunProjectionInvalid, "missing open dispatches")
	}
	dispatches, err := decodeSchema2OpenDispatches(raw)
	if err != nil {
		return result, err
	}
	copyData := cloneRawMap(data)
	copyData["openDispatches"], _ = json.Marshal(dispatches)
	result, err = decodeTypedData[SafeSchema2RunBlockedData](copyData)
	if err != nil {
		return result, projectionError(ErrRunProjectionInvalid, "invalid run_blocked")
	}
	return result, nil
}

func decodeSchema2RunResumedData(data map[string]json.RawMessage) (SafeSchema2RunResumedData, error) {
	var result SafeSchema2RunResumedData
	raw, ok := data["openDispatches"]
	if !ok {
		return result, projectionError(ErrRunProjectionInvalid, "missing open dispatches")
	}
	dispatches, err := decodeSchema2OpenDispatches(raw)
	if err != nil {
		return result, err
	}
	copyData := cloneRawMap(data)
	copyData["openDispatches"], _ = json.Marshal(dispatches)
	result, err = decodeTypedData[SafeSchema2RunResumedData](copyData)
	if err != nil {
		return result, projectionError(ErrRunProjectionInvalid, "invalid run_resumed")
	}
	return result, nil
}

func decodeArtifactProjection(raw json.RawMessage) (ArtifactProjection, error) {
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return nil, projectionError(ErrRunEventUnknown, "invalid artifact projection")
	}
	availability, err := requiredJSONString(object, "availability")
	if err != nil {
		return nil, projectionError(ErrRunEventUnknown, "missing artifact availability")
	}
	if availability == "available" {
		for key := range object {
			if !stringSet("artifactId", "availability", "name", "artifact")[key] {
				return nil, projectionError(ErrRunEventUnknown, "unknown artifact member")
			}
		}
		var result AvailableArtifactProjection
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&result) != nil || result.ArtifactID == "" || result.Artifact.ArtifactID != result.ArtifactID {
			return nil, projectionError(ErrRunEventUnknown, "invalid available artifact")
		}
		return result, nil
	}
	if availability != "unavailable" && availability != "redacted" && availability != "expired" {
		return nil, projectionError(ErrRunEventUnknown, "invalid artifact availability")
	}
	for key := range object {
		if !stringSet("artifactId", "availability", "name", "errorCode")[key] {
			return nil, projectionError(ErrRunEventUnknown, "unknown artifact member")
		}
	}
	var result UnavailableArtifactProjection
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || result.ArtifactID == "" {
		return nil, projectionError(ErrRunEventUnknown, "invalid unavailable artifact")
	}
	return result, nil
}

type projectionState struct {
	view            RunView
	board           *BoardDocument
	nodeIndex       map[string]int
	attemptIndex    map[string]int
	gateIndex       map[string]int
	artifactIndex   map[string]int
	dispatches      map[string]SafeSchema2SlotDispatchData
	dispatchSeq     map[string]uint64
	matchedDispatch map[string]bool
	revokedDispatch map[string]uint64
	bindings        map[string]schema2Binding
	health          map[string]SafeSchema2SlotBindingObservedData
	lastBlockJSON   []byte
	terminal        bool
}

type schema2Binding struct {
	BindingID            string
	NodeID               string
	SlotID               string
	AgentID              string
	Harness              string
	SessionTargetID      string
	TargetFingerprint    string
	SessionLineageSHA256 string
}

type schema2BindingsSnapshot struct {
	Schema   uint64
	RunID    string
	BoardID  string
	BoardRev uint64
	Bindings map[string]schema2Binding
}

type schema2AuthorityProjection struct {
	WorkspaceAuthorityID        string
	WorkspaceRootIdentitySHA256 string
	NextAdmissionSeq            uint64
	AdmissionPolicyRef          AdmissionPolicyRef
}

func projectSchema1Run(runID string, documents canonicalDocuments) (CanonicalRunProjection, error) {
	ledger := oneDocument(documents, CanonicalInputRoleSchema1Ledger)
	events, err := decodeProjectionLedger(ledger.Bytes, CanonicalRunSourceSchema1, runID)
	if err != nil {
		return CanonicalRunProjection{}, err
	}
	board, err := parseBoard(oneDocument(documents, CanonicalInputRoleSchema1GraphSnapshot).Bytes)
	if err != nil {
		return CanonicalRunProjection{}, projectionError(ErrRunProjectionInvalid, "invalid graph snapshot")
	}
	if events[0].typeName != RunEventStarted || events[0].envelope.Seq != 1 {
		return CanonicalRunProjection{}, projectionError(ErrRunProjectionInvalid, "ledger does not start with run_started")
	}
	state := newProjectionState(runID, CanonicalRunSourceSchema1, board)
	startSafe, err := sanitizeSchema1Event(events[0])
	if err != nil {
		return CanonicalRunProjection{}, err
	}
	started, ok := startSafe.(SafeSchema1RunStartedEvent)
	if !ok {
		return CanonicalRunProjection{}, projectionError(ErrRunProjectionInvalid, "invalid start arm")
	}
	if err := initializeSchema1Identity(&state, started); err != nil {
		return CanonicalRunProjection{}, err
	}
	firstRecord := firstLedgerRecord(ledger.Bytes)
	state.view.Generation = projectionGeneration(runID, projectionSHA(firstRecord), oneDocument(documents, CanonicalInputRoleSchema1GraphSnapshot).SHA256, oneDocument(documents, CanonicalInputRoleSchema1BindingsSnapshot).SHA256)
	projected := make([]projectedEvent, 0, len(events))
	for _, event := range events {
		if state.terminal {
			return CanonicalRunProjection{}, projectionError(ErrRunProjectionInvalid, "event follows terminal event")
		}
		safe, sanitizeErr := sanitizeSchema1Event(event)
		if sanitizeErr != nil {
			return CanonicalRunProjection{}, sanitizeErr
		}
		if reduceErr := reduceSchema1Event(&state, event, safe); reduceErr != nil {
			return CanonicalRunProjection{}, reduceErr
		}
		projected = append(projected, projectedEvent{scanSeq: event.envelope.Seq, safe: safe})
	}
	finalizeProjectionState(&state)
	projection := CanonicalRunProjection{view: state.view, events: projected, latestSeq: uint64(len(events))}
	if err := validateProjectedEventSizes(projection); err != nil {
		return CanonicalRunProjection{}, err
	}
	return projection, nil
}

func initializeSchema1Identity(state *projectionState, event SafeSchema1RunStartedEvent) error {
	var boardSlug, missionID, beadID string
	var boardRev uint64
	var limits RunLimits
	root := RunRoot{}
	switch data := event.Data.(type) {
	case SafeSchema1RunStartedMissionData:
		boardSlug, boardRev, missionID, beadID, limits = data.BoardSlug, data.BoardRev, data.MissionID, data.BeadID, data.Limits
		root = RunRoot{Kind: "mission", NodeID: missionID}
	case SafeSchema1RunStartedFormationData:
		boardSlug, boardRev, missionID, beadID, limits = data.BoardSlug, data.BoardRev, data.MissionID, data.BeadID, data.Limits
		root = RunRoot{Kind: "formation", NodeID: data.FormationID}
	default:
		return projectionError(ErrRunProjectionInvalid, "invalid start data union")
	}
	if state.board == nil || state.board.ID != event.BoardID || state.board.Slug != boardSlug || uint64(state.board.Rev) != boardRev {
		return projectionError(ErrRunProjectionInvalid, "graph snapshot identity mismatch")
	}
	state.view.Status = "running"
	state.view.Identity = RunIdentity{BoardID: event.BoardID, BoardSlug: boardSlug, BoardRev: boardRev, RunRoot: root, MissionID: missionID, BeadID: beadID, Epoch: event.Epoch, Redact: limits.Redact}
	state.view.Audit = RunAudit{EventSchema: 1, StartSeq: 1}
	return nil
}

func projectSchema2Run(runID string, documents canonicalDocuments) (CanonicalRunProjection, error) {
	ledger := oneDocument(documents, CanonicalInputRoleSchema2Ledger)
	events, err := decodeProjectionLedger(ledger.Bytes, CanonicalRunSourceSchema2, runID)
	if err != nil {
		return CanonicalRunProjection{}, err
	}
	if events[0].typeName != "run_started" {
		return CanonicalRunProjection{}, projectionError(ErrRunProjectionInvalid, "schema-2 ledger does not start with run_started")
	}
	startSafe, err := sanitizeSchema2Event(events[0])
	if err != nil {
		return CanonicalRunProjection{}, err
	}
	started, ok := startSafe.(SafeSchema2RunStartedEvent)
	if !ok {
		return CanonicalRunProjection{}, projectionError(ErrRunProjectionInvalid, "invalid schema-2 start arm")
	}
	graphDocument := oneDocument(documents, CanonicalInputRoleSchema2GraphSnapshot)
	board, err := parseBoard(graphDocument.Bytes)
	if err != nil {
		return CanonicalRunProjection{}, projectionError(ErrRunProjectionInvalid, "invalid graph snapshot")
	}
	if err := validateSchema2GraphSnapshot(graphDocument.Bytes, board, started); err != nil {
		return CanonicalRunProjection{}, err
	}
	bindingsDocument := oneDocument(documents, CanonicalInputRoleSchema2PrivateBindings)
	bindings, err := parseSchema2Bindings(bindingsDocument.Bytes)
	if err != nil {
		return CanonicalRunProjection{}, err
	}
	if err := validateSchema2BindingsSnapshot(bindings, runID, board, started); err != nil {
		return CanonicalRunProjection{}, err
	}
	authority, commands, err := validateSchema2Documents(runID, documents, events, started)
	if err != nil {
		return CanonicalRunProjection{}, err
	}
	state := newProjectionState(runID, CanonicalRunSourceSchema2, board)
	state.bindings = bindings.Bindings
	bootstrap, err := decodeUniqueJSONObject(oneDocument(documents, CanonicalInputRoleSchema2RunBootstrap).Bytes)
	if err != nil {
		return CanonicalRunProjection{}, projectionError(ErrRunProjectionInvalid, "invalid run bootstrap")
	}
	bootstrapRunID, _ := requiredJSONString(bootstrap, "runId")
	authorityID, _ := requiredJSONString(bootstrap, "runAuthorityId")
	graphHash, _ := requiredJSONString(bootstrap, "graphSnapshotSha256")
	bindingsHash, _ := requiredJSONString(bootstrap, "privateBindingsSha256")
	if bootstrapRunID != runID || authorityID != started.Data.RunAuthorityID || graphHash != graphDocument.SHA256 || bindingsHash != bindingsDocument.SHA256 || started.Data.GraphSnapshotSHA256 != graphHash || started.Data.PrivateBindingsSHA256 != bindingsHash {
		return CanonicalRunProjection{}, projectionError(ErrRunProjectionInvalid, "schema-2 immutable identity mismatch")
	}
	if started.Data.WorkspaceAuthorityID != authority.WorkspaceAuthorityID || started.Data.AdmissionPolicyRev != authority.AdmissionPolicyRef.PolicyRev || started.Data.AdmissionPolicySHA256 != authority.AdmissionPolicyRef.PolicySHA256 {
		return CanonicalRunProjection{}, projectionError(ErrRunProjectionInvalid, "schema-2 policy mismatch")
	}
	state.view.Generation = projectionGeneration(runID, authorityID, graphHash, bindingsHash, started.Data.AdmissionCommandID)
	state.view.Status = "queued"
	state.view.Identity = RunIdentity{BoardID: started.BoardID, BoardSlug: started.Data.BoardSlug, BoardRev: started.BoardRev, RunRoot: started.Data.RunRoot, MissionID: started.MissionID, BeadID: started.BeadID, Epoch: started.Epoch, Redact: started.Data.Limits.Redact}
	authoritySchema := uint64(2)
	state.view.Audit = RunAudit{EventSchema: 2, AuthoritySchema: &authoritySchema, StartSeq: 1, AdmissionCommandID: started.Data.AdmissionCommandID, CommandPayloadSHA256: started.Data.CommandPayloadSHA256, WorkspaceAdmissionSeq: started.Data.WorkspaceAdmissionSeq, AdmissionPolicyRev: authority.AdmissionPolicyRef.PolicyRev, AdmissionPolicySHA256: authority.AdmissionPolicyRef.PolicySHA256, GraphSnapshotSHA256: graphHash, BindingProjectionSHA256: started.Data.BindingProjectionSHA256}
	projected := make([]projectedEvent, 0, len(events))
	for _, event := range events {
		if state.terminal {
			return CanonicalRunProjection{}, projectionError(ErrRunProjectionInvalid, "event follows terminal event")
		}
		safe, sanitizeErr := sanitizeSchema2Event(event)
		if sanitizeErr != nil {
			return CanonicalRunProjection{}, sanitizeErr
		}
		if reduceErr := reduceSchema2Event(&state, event, safe, commands); reduceErr != nil {
			return CanonicalRunProjection{}, reduceErr
		}
		projected = append(projected, projectedEvent{scanSeq: event.envelope.Seq, safe: safe})
	}
	finalizeProjectionState(&state)
	for index, item := range projected {
		attached, ok := item.safe.(SafeSchema2ArtifactAttachedEvent)
		if !ok {
			continue
		}
		artifactID := artifactProjectionID(attached.Data.ArtifactProjection)
		if latest, found := artifactByID(state.view.Artifacts, artifactID); found {
			attached.Data.ArtifactProjection = latest
			projected[index].safe = attached
		}
	}
	projection := CanonicalRunProjection{view: state.view, events: projected, latestSeq: uint64(len(events))}
	if err := validateProjectedEventSizes(projection); err != nil {
		return CanonicalRunProjection{}, err
	}
	return projection, nil
}

func newProjectionState(runID string, source CanonicalRunSource, board *BoardDocument) projectionState {
	sourceProjection := CanonicalRunSourceProjection{EventSchema: 1, Compatibility: true}
	if source == CanonicalRunSourceSchema2 {
		authority := uint64(2)
		sourceProjection = CanonicalRunSourceProjection{EventSchema: 2, AuthoritySchema: &authority, Compatibility: false}
	}
	state := projectionState{
		view:  RunView{Schema: RunViewSchema, RunID: runID, Source: sourceProjection, RecoveryState: "live", Nodes: []RunNodeView{}, Attempts: []RunAttemptView{}, Gates: []RunGateView{}, Outputs: []RunOutputView{}, Artifacts: []ArtifactProjection{}, Blocks: []RunBlockView{}, Escalations: []RunEscalationView{}, Sessions: []RunSessionView{}, Actions: []RunAction{}},
		board: board, nodeIndex: map[string]int{}, attemptIndex: map[string]int{}, gateIndex: map[string]int{}, artifactIndex: map[string]int{}, dispatches: map[string]SafeSchema2SlotDispatchData{}, dispatchSeq: map[string]uint64{}, matchedDispatch: map[string]bool{}, revokedDispatch: map[string]uint64{}, bindings: map[string]schema2Binding{}, health: map[string]SafeSchema2SlotBindingObservedData{},
	}
	appendNode := func(id, kind string) {
		if id == "" {
			return
		}
		state.nodeIndex[id] = len(state.view.Nodes)
		state.view.Nodes = append(state.view.Nodes, RunNodeView{NodeID: id, Kind: kind, Status: "not_run", Readiness: RunReadiness{WaitingFor: []string{}}, Attempts: []RunAttemptRef{}, Outputs: []RunOutputRef{}, Gates: []RunGateRef{}, Sessions: []RunSessionRef{}})
	}
	if board != nil {
		for _, node := range board.Missions {
			appendNode(node.ID, "mission")
		}
		for _, node := range board.Formations {
			appendNode(node.ID, "formation")
		}
		for _, node := range board.Tools {
			appendNode(node.ID, "tool")
		}
		for _, node := range board.Gates {
			appendNode(node.ID, "gate")
		}
	}
	return state
}

func reduceSchema1Event(state *projectionState, raw rawProjectionEvent, safe SafeRunEvent) error {
	state.view.Cursor = raw.envelope.Seq
	state.view.Audit.ConsumedEventCount++
	state.view.Identity.Epoch = raw.envelope.Epoch
	switch event := safe.(type) {
	case SafeSchema1RunStartedEvent:
		if raw.envelope.Seq != 1 {
			return projectionError(ErrRunProjectionInvalid, "late run_started")
		}
	case SafeSchema1NodeWaitingEvent:
		node := state.node(raw.envelope.NodeID)
		if node == nil {
			return projectionError(ErrRunProjectionInvalid, "unknown waiting node")
		}
		node.Status = "waiting"
		node.Readiness = RunReadiness{NeededInputs: event.Data.NeededInputs, ReadyInputs: event.Data.ReadyInputs, TotalInputs: event.Data.TotalInputs, WaitingFor: append([]string(nil), event.Data.WaitingFor...)}
	case SafeSchema1NodeStartedEvent:
		state.startAttempt(raw.envelope.NodeID, raw.envelope.Attempt, raw.envelope.Seq, event.Data.InputRefs)
	case SafeSchema1NodeOutputEvent:
		if err := state.completeSchema1Node(raw, event.Data); err != nil {
			return err
		}
	case SafeSchema1GateEvaluatingEvent:
		state.startGate(raw.envelope.GateID, raw.envelope.Attempt, raw.envelope.Seq, event.Data.InputRef)
	case SafeSchema1GateVerdictEvent:
		state.finishGate(raw.envelope.GateID, raw.envelope.Attempt, raw.envelope.Seq, event.Data.Verdict, event.Data.Reason)
	case SafeSchema1HumanInputRequestedEvent:
		gateID := raw.envelope.GateID
		if gateID == "" {
			gateID = event.Data.GateID
		}
		attempt := raw.envelope.Attempt
		if attempt == 0 {
			attempt = 1
			if node := state.node(gateID); node != nil && node.LatestAttempt != 0 {
				attempt = node.LatestAttempt
			}
		}
		gate := state.ensureGate(gateID, attempt)
		if gate == nil {
			return projectionError(ErrRunProjectionInvalid, "unknown human gate")
		}
		gate.Status = "waiting_human"
		gate.RequestSeq = raw.envelope.Seq
		state.view.Status = "waiting_human"
	case SafeSchema1EscalationRaisedEvent:
		state.view.Escalations = append(state.view.Escalations, RunEscalationView{Seq: raw.envelope.Seq, NodeID: emptyToZero(event.Data.NodeID), GateID: emptyToZero(event.Data.GateID), Severity: event.Data.Severity, Reason: event.Data.Reason, Source: event.Data.Source, Trigger: event.Data.Trigger, Blocks: event.Data.Blocks})
	case SafeSchema1RunBlockedEvent:
		state.view.Status = "blocked"
		state.appendSchema1Block(raw, event.Data)
	case SafeSchema1RunResumedEvent:
		if len(state.view.Blocks) == 0 {
			return projectionError(ErrRunProjectionInvalid, "resume without block")
		}
		carry, _ := json.Marshal(event.Data.OpenDispatches)
		if !bytes.Equal(carry, state.lastBlockJSON) {
			return projectionError(ErrRunProjectionInvalid, "resume carry differs from block")
		}
		state.view.Status = "running"
	case SafeSchema1RunCanceledEvent:
		state.finishRun("canceled", "canceled", raw.envelope.Seq)
	case SafeSchema1RunFailedEvent:
		state.finishRun("failed", "failed", raw.envelope.Seq)
	case SafeSchema1RunSucceededEvent:
		state.view.Status = "succeeded"
		state.view.Final = true
		state.terminal = true
	}
	return nil
}

func reduceSchema2Event(state *projectionState, raw rawProjectionEvent, safe SafeRunEvent, commands map[string]RunCommandReceipt) error {
	state.view.Cursor = raw.envelope.Seq
	state.view.Audit.ConsumedEventCount++
	state.view.Audit.LatestWriterFence = raw.writerFence
	state.view.Identity.Epoch = raw.envelope.Epoch
	switch event := safe.(type) {
	case SafeSchema2RunStartedEvent:
		if err := verifyEventCommand(event.Data.AdmissionCommandID, event.Data.CommandPayloadSHA256, raw.envelope.Seq, commands); err != nil {
			return err
		}
	case SafeSchema2RunActivatedEvent:
		if event.Data.AdmissionPolicyRev != state.view.Audit.AdmissionPolicyRev || event.Data.AdmissionPolicySHA256 != state.view.Audit.AdmissionPolicySHA256 {
			return projectionError(ErrRunProjectionInvalid, "activation policy mismatch")
		}
		state.view.Status = "running"
		state.view.Audit.ActivationPolicyRev = event.Data.AdmissionPolicyRev
		state.view.Audit.ActivationPolicySHA256 = event.Data.AdmissionPolicySHA256
	case SafeSchema2NodeWaitingEvent:
		node := state.node(raw.envelope.NodeID)
		if node == nil {
			return projectionError(ErrRunProjectionInvalid, "unknown waiting node")
		}
		node.Status = "waiting"
		node.Readiness = RunReadiness{NeededInputs: event.Data.NeededInputs, ReadyInputs: event.Data.ReadyInputs, TotalInputs: event.Data.TotalInputs, WaitingFor: append([]string(nil), event.Data.WaitingFor...)}
	case SafeSchema2NodeStartedEvent:
		state.startAttempt(event.Data.NodeID, event.Data.Attempt, raw.envelope.Seq, event.Data.InputRefs)
	case SafeSchema2SlotBindingObservedEvent:
		state.health[event.Data.BindingID] = event.Data
	case SafeSchema2SlotDispatchEvent:
		if err := state.addSchema2Dispatch(raw, event.Data); err != nil {
			return err
		}
	case SafeSchema2SlotPeekCapabilityRevokedEvent:
		state.revokedDispatch[event.Data.DispatchID] = raw.envelope.Seq
		state.setSessionCapability(event.Data.DispatchID, "revoked", event.Data.CapabilityIssuedSeq, event.Data.CapabilityGeneration)
	case SafeSchema2SlotResultEvent:
		state.matchedDispatch[event.Data.DispatchID] = true
		state.setSessionOperatorInfluenced(event.Data.DispatchID, event.Data.OperatorInfluenced)
	case SafeSchema2ArtifactAttachedEvent:
		state.upsertArtifact(event.Data.ArtifactProjection)
	case SafeSchema2ArtifactObservedEvent:
		state.observeArtifact(event.Data)
	case SafeSchema2RunBlockedEvent:
		if err := state.validateSchema2OpenDispatches(event.Data.OpenDispatches); err != nil {
			return err
		}
		state.view.Status = "blocked"
		state.appendSchema2Block(raw, event.Data)
	case SafeSchema2RunResumedEvent:
		if err := verifyEventCommand(event.Data.CommandID, event.Data.CommandPayloadSHA256, raw.envelope.Seq, commands); err != nil {
			return err
		}
		carry, _ := json.Marshal(event.Data.OpenDispatches)
		if !bytes.Equal(carry, state.lastBlockJSON) {
			return projectionError(ErrRunProjectionInvalid, "resume carry differs from block")
		}
		state.view.Status = "running"
	case SafeSchema2HumanVerdictRecordedEvent:
		if err := verifyEventCommand(event.Data.CommandID, event.Data.CommandPayloadSHA256, raw.envelope.Seq, commands); err != nil {
			return err
		}
	case SafeSchema2RunCancelRequestedEvent:
		if err := verifyEventCommand(event.Data.CommandID, event.Data.CommandPayloadSHA256, raw.envelope.Seq, commands); err != nil {
			return err
		}
		state.view.Status = "canceling"
	case SafeSchema2RunFailureReconciliationStartedEvent:
		state.view.Status = "failing"
	case SafeSchema2RunCanceledEvent:
		state.finishRun("canceled", "canceled", raw.envelope.Seq)
	case SafeSchema2RunFailedEvent:
		state.finishRun("failed", "failed", raw.envelope.Seq)
	case SafeSchema2RunSucceededEvent:
		state.view.Status = "succeeded"
		state.view.Final = true
		state.terminal = true
	}
	return nil
}

func (state *projectionState) node(nodeID string) *RunNodeView {
	index, ok := state.nodeIndex[nodeID]
	if !ok {
		return nil
	}
	return &state.view.Nodes[index]
}

func projectionAttemptKey(nodeID string, attempt uint64) string {
	return nodeID + "/" + strconv.FormatUint(attempt, 10)
}

func (state *projectionState) startAttempt(nodeID string, attempt, sequence uint64, inputs []SafeInputIdentity) *RunAttemptView {
	node := state.node(nodeID)
	if node == nil || attempt == 0 || attempt > MaxJSONSafeInteger {
		return nil
	}
	key := projectionAttemptKey(nodeID, attempt)
	if index, exists := state.attemptIndex[key]; exists {
		return &state.view.Attempts[index]
	}
	copyInputs := append([]SafeInputIdentity(nil), inputs...)
	state.attemptIndex[key] = len(state.view.Attempts)
	state.view.Attempts = append(state.view.Attempts, RunAttemptView{
		NodeID: nodeID, Attempt: attempt, Status: "running", StartedSeq: sequence,
		InputRefs: copyInputs, Slots: []RunSessionRef{}, Outputs: []RunOutputRef{},
	})
	node.Status = "running"
	node.LatestAttempt = attempt
	node.Readiness = RunReadiness{NeededInputs: uint64(len(inputs)), ReadyInputs: uint64(len(inputs)), TotalInputs: uint64(len(inputs)), WaitingFor: []string{}}
	return &state.view.Attempts[len(state.view.Attempts)-1]
}

func (state *projectionState) ensureAttempt(nodeID string, attempt uint64) *RunAttemptView {
	key := projectionAttemptKey(nodeID, attempt)
	if index, ok := state.attemptIndex[key]; ok {
		return &state.view.Attempts[index]
	}
	return state.startAttempt(nodeID, attempt, 0, []SafeInputIdentity{})
}

func (state *projectionState) completeSchema1Node(raw rawProjectionEvent, data SafeSchema1NodeOutputData) error {
	node := state.node(raw.envelope.NodeID)
	if node == nil {
		return projectionError(ErrRunProjectionInvalid, "unknown output node")
	}
	attemptNumber := raw.envelope.Attempt
	if attemptNumber == 0 {
		attemptNumber = node.LatestAttempt
		if attemptNumber == 0 {
			attemptNumber = 1
		}
	}
	attempt := state.ensureAttempt(raw.envelope.NodeID, attemptNumber)
	if attempt == nil {
		return projectionError(ErrRunProjectionInvalid, "unknown output attempt for node %q raw=%d latest=%d", raw.envelope.NodeID, raw.envelope.Attempt, node.LatestAttempt)
	}
	status := data.Status
	if status == "" {
		status = "done"
	}
	node.Status, node.FinalDisposition = status, status
	attempt.Status, attempt.Disposition, attempt.CompletedSeq = status, status, raw.envelope.Seq
	portIDs := state.outputPortIDs(raw.envelope.NodeID)
	seen := map[string]bool{}
	for _, portID := range portIDs {
		value, ok := data.Outputs[portID]
		if !ok {
			continue
		}
		seen[portID] = true
		state.appendOutput(raw.envelope.NodeID, attemptNumber, portID, raw.envelope.Seq, value.Text)
	}
	var extras []string
	for portID := range data.Outputs {
		if !seen[portID] {
			extras = append(extras, portID)
		}
	}
	sort.Strings(extras)
	for _, portID := range extras {
		state.appendOutput(raw.envelope.NodeID, attemptNumber, portID, raw.envelope.Seq, data.Outputs[portID].Text)
	}
	return nil
}

func (state *projectionState) outputPortIDs(nodeID string) []string {
	if state.board == nil {
		return nil
	}
	for _, node := range state.board.Formations {
		if node.ID == nodeID {
			result := make([]string, len(node.Outputs))
			for index := range node.Outputs {
				result[index] = node.Outputs[index].ID
			}
			return result
		}
	}
	for _, node := range state.board.Tools {
		if node.ID == nodeID {
			result := make([]string, len(node.Outputs))
			for index := range node.Outputs {
				result[index] = node.Outputs[index].ID
			}
			return result
		}
	}
	return nil
}

func (state *projectionState) appendOutput(nodeID string, attempt uint64, portID string, sequence uint64, text string) {
	state.view.Outputs = append(state.view.Outputs, RunOutputView{
		NodeID: nodeID, Attempt: attempt, PortID: portID, OutcomeSeq: sequence,
		PayloadProjection: PayloadProjection{Availability: "available", Exact: true, Payload: PayloadValue{Kind: "work", MediaType: "text/plain", Text: text}},
	})
}

func (state *projectionState) startGate(gateID string, attempt, sequence uint64, _ SafeInputIdentity) {
	node := state.node(gateID)
	if node == nil {
		return
	}
	owner := state.ensureAttempt(gateID, attempt)
	if owner == nil {
		return
	}
	key := projectionAttemptKey(gateID, attempt)
	if _, exists := state.gateIndex[key]; exists {
		return
	}
	state.gateIndex[key] = len(state.view.Gates)
	state.view.Gates = append(state.view.Gates, RunGateView{GateID: gateID, Attempt: attempt, Status: "evaluating", EvaluatingSeq: sequence, Evidence: []SafeGateEvidence{}})
	node.Status = "evaluating"
}

func (state *projectionState) ensureGate(gateID string, attempt uint64) *RunGateView {
	key := projectionAttemptKey(gateID, attempt)
	if index, ok := state.gateIndex[key]; ok {
		return &state.view.Gates[index]
	}
	state.startGate(gateID, attempt, 0, SafeInputIdentity{})
	if index, ok := state.gateIndex[key]; ok {
		return &state.view.Gates[index]
	}
	return nil
}

func (state *projectionState) finishGate(gateID string, attempt, sequence uint64, verdict, reason string) {
	gate := state.ensureGate(gateID, attempt)
	if gate == nil {
		return
	}
	gate.VerdictSeq, gate.Verdict, gate.Reason = sequence, verdict, reason
	if verdict == "pass" {
		gate.Status = "passed"
	} else {
		gate.Status = "failed"
	}
	if node := state.node(gateID); node != nil {
		node.Status, node.FinalDisposition = gate.Status, gate.Status
	}
	if attemptView := state.ensureAttempt(gateID, attempt); attemptView != nil {
		attemptView.Status, attemptView.Disposition, attemptView.CompletedSeq = gate.Status, gate.Status, sequence
	}
}

func (state *projectionState) appendSchema1Block(raw rawProjectionEvent, data SafeSchema1RunBlockedData) {
	dispatches := make([]SafeOpenDispatch, len(data.OpenDispatches))
	for index := range data.OpenDispatches {
		dispatches[index] = data.OpenDispatches[index]
	}
	scope := "run"
	if data.BlockedNodeID != "" {
		scope = "node"
	} else if data.BlockedGateID != "" {
		scope = "gate"
	}
	state.view.Blocks = append(state.view.Blocks, RunBlockView{
		Seq: raw.envelope.Seq, Epoch: raw.envelope.Epoch, Scope: scope, NodeID: data.BlockedNodeID,
		GateID: data.BlockedGateID, Code: data.Code, Reason: data.Reason, ResumeAllowed: data.ResumeAllowed,
		ResumePolicy: data.ResumePolicy, NextEpoch: data.NextEpoch, OpenDispatches: dispatches,
	})
	state.lastBlockJSON, _ = json.Marshal(data.OpenDispatches)
}

type schema2AuthoredConfigManifest struct {
	Classification string
	SourceKind     string
	NodeID         string
	Encoding       string
	MediaType      string
	SHA256         string
}

func validateSchema2GraphSnapshot(raw []byte, board *BoardDocument, started SafeSchema2RunStartedEvent) error {
	if board == nil || board.Schema != 2 || board.ID != started.BoardID || board.Slug != started.Data.BoardSlug || uint64(board.Rev) != started.BoardRev {
		return projectionError(ErrRunProjectionInvalid, "graph snapshot identity mismatch")
	}
	topLevelKeys := stringSet("schema", "id", "slug", "title", "rev", "updatedBy", "updatedAt")
	sectionKeys := map[string]map[string]bool{
		"mission":                stringSet("id", "title", "goal", "beadId"),
		"authoredConfigManifest": stringSet("classification", "sourceKind", "nodeId", "encoding", "mediaType", "sha256"),
		"formation":              stringSet("id", "type", "title", "verification", "verification.id", "verification.kinds", "verification.criterion", "verification.onFail"),
		"formation.input":        stringSet("id", "label"),
		"formation.output":       stringSet("id", "label"),
		"formation.slot":         stringSet("id", "label", "agentId", "harness", "controller"),
		"formation.brief":        stringSet("goal", "beadId", "files", "links"),
		"formation.verification": stringSet("id", "kinds", "criterion", "onFail"),
		"gate":                   stringSet("id", "title", "kinds", "criterion", "command", "commandArgv", "commandCwd", "commandShell", "judgeChain"),
		"connection":             stringSet("id", "from", "to"),
		"tool":                   stringSet("id", "title", "profileId", "profileVersion"),
		"tool.input":             stringSet("id", "name", "label", "direction", "kind", "acceptedMediaTypes", "required", "role"),
		"tool.output":            stringSet("id", "name", "label", "direction", "kind", "acceptedMediaTypes", "required", "role"),
		"tool.params":            nil,
	}
	arraySections := stringSet("mission", "authoredConfigManifest", "formation", "formation.input", "formation.output", "formation.slot", "gate", "connection", "tool", "tool.input", "tool.output")
	seenTop := map[string]bool{}
	seenSection := map[string]bool{}
	section := ""
	var manifests []schema2AuthoredConfigManifest
	var manifest *schema2AuthoredConfigManifest
	for _, line := range splitLines(raw) {
		trimmed := strings.TrimSpace(line.body)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || line.valueContinuation {
			continue
		}
		name, isSection := tomlLineSectionName(line)
		if isSection {
			allowed, exists := sectionKeys[name]
			_ = allowed
			isArray := strings.HasPrefix(trimmed, "[[")
			if !exists || isArray != arraySections[name] {
				return projectionError(ErrRunProjectionInvalid, "unknown graph snapshot section")
			}
			section = name
			seenSection = map[string]bool{}
			manifest = nil
			if name == "authoredConfigManifest" {
				manifests = append(manifests, schema2AuthoredConfigManifest{})
				manifest = &manifests[len(manifests)-1]
			}
			continue
		}
		if isTOMLHeader(line) {
			return projectionError(ErrRunProjectionInvalid, "invalid graph snapshot section")
		}
		key, value, ok := tomlKeyValue(line.body)
		if !ok {
			return projectionError(ErrRunProjectionInvalid, "malformed graph snapshot line")
		}
		if section == "" {
			if !topLevelKeys[key] || seenTop[key] {
				return projectionError(ErrRunProjectionInvalid, "invalid graph snapshot top-level member")
			}
			seenTop[key] = true
			continue
		}
		allowed := sectionKeys[section]
		if (allowed != nil && !allowed[key]) || seenSection[key] {
			return projectionError(ErrRunProjectionInvalid, "invalid graph snapshot member")
		}
		seenSection[key] = true
		if manifest != nil {
			switch key {
			case "classification":
				manifest.Classification = parseString(value)
			case "sourceKind":
				manifest.SourceKind = parseString(value)
			case "nodeId":
				manifest.NodeID = parseString(value)
			case "encoding":
				manifest.Encoding = parseString(value)
			case "mediaType":
				manifest.MediaType = parseString(value)
			case "sha256":
				manifest.SHA256 = parseString(value)
			}
		}
	}
	if len(seenTop) < 5 || !graphContainsRunRoot(board, started.Data.RunRoot) {
		return projectionError(ErrRunProjectionInvalid, "graph snapshot root mismatch")
	}
	root := started.Data.RootInputProjection
	if root.Classification != "authored_config" || !lowercaseProjectionSHA(root.SHA256) || projectionSHA([]byte(root.Text)) != root.SHA256 {
		return projectionError(ErrRunProjectionInvalid, "invalid root input projection")
	}
	for _, candidate := range manifests {
		if candidate.NodeID == started.Data.RunRoot.NodeID && candidate.Classification == root.Classification && candidate.SourceKind == root.SourceKind && candidate.Encoding == root.Encoding && candidate.MediaType == root.MediaType && candidate.SHA256 == root.SHA256 {
			return nil
		}
	}
	return projectionError(ErrRunProjectionInvalid, "authored config manifest mismatch")
}

func graphContainsRunRoot(board *BoardDocument, root RunRoot) bool {
	switch root.Kind {
	case "mission":
		for _, node := range board.Missions {
			if node.ID == root.NodeID {
				return true
			}
		}
	case "formation":
		for _, node := range board.Formations {
			if node.ID == root.NodeID {
				return true
			}
		}
	}
	return false
}

func validateSchema2BindingsSnapshot(snapshot schema2BindingsSnapshot, runID string, board *BoardDocument, started SafeSchema2RunStartedEvent) error {
	if snapshot.Schema != 2 || snapshot.RunID != runID || snapshot.BoardID != started.BoardID || snapshot.BoardID != board.ID || snapshot.BoardRev != started.BoardRev || snapshot.BoardRev != uint64(board.Rev) {
		return projectionError(ErrRunProjectionInvalid, "schema-2 bindings identity mismatch")
	}
	seenTargets := map[string]bool{}
	for _, binding := range snapshot.Bindings {
		var slot *FormationSlot
		for formationIndex := range board.Formations {
			formation := &board.Formations[formationIndex]
			if formation.ID != binding.NodeID {
				continue
			}
			for slotIndex := range formation.Slots {
				if formation.Slots[slotIndex].ID == binding.SlotID {
					slot = &formation.Slots[slotIndex]
					break
				}
			}
		}
		if slot == nil || slot.AgentID != binding.AgentID || slot.Harness != binding.Harness || seenTargets[binding.SessionTargetID] {
			return projectionError(ErrRunProjectionInvalid, "schema-2 binding graph mismatch")
		}
		seenTargets[binding.SessionTargetID] = true
	}
	return nil
}

type projectionWorkspaceBootstrap struct {
	WorkspaceAuthorityID        string
	WorkspaceRootIdentitySHA256 string
}

type projectionRunBootstrap struct {
	WorkspaceAuthorityID string
	RunID                string
}

func closedProjectionJSONObject(raw []byte, required []string, optional ...string) (map[string]json.RawMessage, error) {
	object, err := decodeUniqueJSONObject(raw)
	if err != nil {
		return nil, err
	}
	allowed := stringSet(append(append([]string(nil), required...), optional...)...)
	for key := range object {
		if !allowed[key] {
			return nil, errors.New("unknown JSON member")
		}
	}
	for _, key := range required {
		if _, ok := object[key]; !ok {
			return nil, errors.New("missing JSON member")
		}
	}
	return object, nil
}

func validateProjectionPriorGeneration(raw json.RawMessage, recordRev uint64) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if recordRev != 1 {
			return errors.New("missing predecessor")
		}
		return nil
	}
	object, err := closedProjectionJSONObject(raw, []string{"recordRev", "sha256"})
	if err != nil {
		return err
	}
	priorRev, revErr := requiredJSONSafeUint(object, "recordRev")
	priorSHA, shaErr := requiredJSONString(object, "sha256")
	if revErr != nil || shaErr != nil || priorRev == 0 || priorRev+1 != recordRev || !lowercaseProjectionSHA(priorSHA) {
		return errors.New("invalid predecessor")
	}
	return nil
}

func validateProjectionWorkspaceRegistry(raw []byte) (map[string]string, error) {
	object, err := closedProjectionJSONObject(raw, []string{"registrySchema", "recordRev", "priorGeneration", "entries"})
	if err != nil {
		return nil, projectionError(ErrRunProjectionInvalid, "invalid workspace registry")
	}
	schema, schemaErr := requiredJSONSafeUint(object, "registrySchema")
	recordRev, revErr := requiredJSONSafeUint(object, "recordRev")
	if schemaErr != nil || revErr != nil || schema != 1 || recordRev == 0 || validateProjectionPriorGeneration(object["priorGeneration"], recordRev) != nil {
		return nil, projectionError(ErrRunProjectionInvalid, "invalid workspace registry")
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(object["entries"], &entries); err != nil || len(entries) == 0 {
		return nil, projectionError(ErrRunProjectionInvalid, "invalid workspace registry entries")
	}
	result := make(map[string]string, len(entries))
	seenPaths, seenOpened, seenRoots := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, rawEntry := range entries {
		entry, entryErr := closedProjectionJSONObject(rawEntry, []string{"workspaceAuthorityId", "configuredPath", "device", "inode", "workspaceRootIdentitySha256"})
		if entryErr != nil {
			return nil, projectionError(ErrRunProjectionInvalid, "invalid workspace registry entry")
		}
		authorityID, idErr := requiredJSONString(entry, "workspaceAuthorityId")
		configuredPath, pathErr := requiredJSONString(entry, "configuredPath")
		device, deviceErr := requiredJSONString(entry, "device")
		inode, inodeErr := requiredJSONString(entry, "inode")
		rootHash, rootErr := requiredJSONString(entry, "workspaceRootIdentitySha256")
		_, canonicalDeviceErr := runtimeCanonicalUint64String(device)
		_, canonicalInodeErr := runtimeCanonicalUint64String(inode)
		opened := device + ":" + inode
		if idErr != nil || pathErr != nil || deviceErr != nil || inodeErr != nil || rootErr != nil || !safeProjectionIdentifier(authorityID) || validateRuntimeConfiguredPath(configuredPath) != nil || canonicalDeviceErr != nil || canonicalInodeErr != nil || !lowercaseProjectionSHA(rootHash) || seenPaths[configuredPath] || seenOpened[opened] || seenRoots[rootHash] || result[authorityID] != "" {
			return nil, projectionError(ErrRunProjectionInvalid, "invalid workspace registry entry")
		}
		seenPaths[configuredPath], seenOpened[opened], seenRoots[rootHash] = true, true, true
		result[authorityID] = rootHash
	}
	return result, nil
}

func validateProjectionWorkspaceBootstrap(raw []byte) (projectionWorkspaceBootstrap, error) {
	object, err := closedProjectionJSONObject(raw, []string{"bootstrapSchema", "workspaceAuthorityId", "rootIdentityEncoding", "workspaceRootIdentitySha256"})
	if err != nil {
		return projectionWorkspaceBootstrap{}, projectionError(ErrRunProjectionInvalid, "invalid workspace bootstrap")
	}
	schema, schemaErr := requiredJSONSafeUint(object, "bootstrapSchema")
	authorityID, idErr := requiredJSONString(object, "workspaceAuthorityId")
	encoding, encodingErr := requiredJSONString(object, "rootIdentityEncoding")
	rootHash, rootErr := requiredJSONString(object, "workspaceRootIdentitySha256")
	if schemaErr != nil || idErr != nil || encodingErr != nil || rootErr != nil || schema != 1 || !safeProjectionIdentifier(authorityID) || encoding != "workspace-root-identity-v1" || !lowercaseProjectionSHA(rootHash) {
		return projectionWorkspaceBootstrap{}, projectionError(ErrRunProjectionInvalid, "invalid workspace bootstrap")
	}
	return projectionWorkspaceBootstrap{WorkspaceAuthorityID: authorityID, WorkspaceRootIdentitySHA256: rootHash}, nil
}

func validateProjectionWorkspaceAuthority(raw []byte) (schema2AuthorityProjection, error) {
	object, err := closedProjectionJSONObject(raw, []string{"recordRev", "priorGeneration", "authoritySchema", "workspaceAuthorityId", "rootIdentityEncoding", "workspaceRootIdentitySha256", "nextWriterFence", "nextAdmissionSeq", "admissionPolicyRef"})
	if err != nil {
		return schema2AuthorityProjection{}, projectionError(ErrRunProjectionInvalid, "invalid workspace authority")
	}
	recordRev, revErr := requiredJSONSafeUint(object, "recordRev")
	schema, schemaErr := requiredJSONSafeUint(object, "authoritySchema")
	authorityID, idErr := requiredJSONString(object, "workspaceAuthorityId")
	encoding, encodingErr := requiredJSONString(object, "rootIdentityEncoding")
	rootHash, rootErr := requiredJSONString(object, "workspaceRootIdentitySha256")
	nextWriterFence, fenceErr := requiredJSONSafeUint(object, "nextWriterFence")
	nextAdmissionSeq, admissionErr := requiredJSONSafeUint(object, "nextAdmissionSeq")
	policyObject, policyErr := closedProjectionJSONObject(object["admissionPolicyRef"], []string{"policyRev", "policySha256"})
	policyRev, policyRevErr := requiredJSONSafeUint(policyObject, "policyRev")
	policySHA, policySHAErr := requiredJSONString(policyObject, "policySha256")
	if revErr != nil || schemaErr != nil || idErr != nil || encodingErr != nil || rootErr != nil || fenceErr != nil || admissionErr != nil || policyErr != nil || policyRevErr != nil || policySHAErr != nil || recordRev == 0 || schema != 2 || !safeProjectionIdentifier(authorityID) || encoding != "workspace-root-identity-v1" || !lowercaseProjectionSHA(rootHash) || nextWriterFence == 0 || nextAdmissionSeq == 0 || policyRev == 0 || !lowercaseProjectionSHA(policySHA) || validateProjectionPriorGeneration(object["priorGeneration"], recordRev) != nil {
		return schema2AuthorityProjection{}, projectionError(ErrRunProjectionInvalid, "invalid workspace authority")
	}
	return schema2AuthorityProjection{WorkspaceAuthorityID: authorityID, WorkspaceRootIdentitySHA256: rootHash, NextAdmissionSeq: nextAdmissionSeq, AdmissionPolicyRef: AdmissionPolicyRef{PolicyRev: policyRev, PolicySHA256: policySHA}}, nil
}

func validateProjectionRunBootstrap(raw []byte) (projectionRunBootstrap, error) {
	object, err := closedProjectionJSONObject(raw, []string{"runBootstrapSchema", "workspaceAuthorityId", "runId", "runAuthorityId", "graphSnapshotEncoding", "graphSnapshotSha256", "privateBindingsEncoding", "privateBindingsSha256"})
	if err != nil {
		return projectionRunBootstrap{}, err
	}
	schema, schemaErr := requiredJSONSafeUint(object, "runBootstrapSchema")
	workspaceID, workspaceErr := requiredJSONString(object, "workspaceAuthorityId")
	runID, runErr := requiredJSONString(object, "runId")
	authorityID, authorityErr := requiredJSONString(object, "runAuthorityId")
	graphEncoding, graphEncodingErr := requiredJSONString(object, "graphSnapshotEncoding")
	graphHash, graphHashErr := requiredJSONString(object, "graphSnapshotSha256")
	bindingsEncoding, bindingsEncodingErr := requiredJSONString(object, "privateBindingsEncoding")
	bindingsHash, bindingsHashErr := requiredJSONString(object, "privateBindingsSha256")
	if schemaErr != nil || workspaceErr != nil || runErr != nil || authorityErr != nil || graphEncodingErr != nil || graphHashErr != nil || bindingsEncodingErr != nil || bindingsHashErr != nil || schema != 1 || !safeProjectionIdentifier(workspaceID) || !validRunID(runID) || !safeProjectionIdentifier(authorityID) || graphEncoding != "run-graph-snapshot-toml-v1" || bindingsEncoding != "run-private-bindings-toml-v1" || !lowercaseProjectionSHA(graphHash) || !lowercaseProjectionSHA(bindingsHash) {
		return projectionRunBootstrap{}, errors.New("invalid run bootstrap")
	}
	return projectionRunBootstrap{WorkspaceAuthorityID: workspaceID, RunID: runID}, nil
}

func validateProjectionAdmissionPolicy(raw []byte) (uint64, string, error) {
	object, err := closedProjectionJSONObject(raw, []string{"policySchema", "policyRev", "priorPolicySha256", "state"}, "maxActiveRuns", "maxQueuedRuns")
	if err != nil {
		return 0, "", err
	}
	schema, schemaErr := requiredJSONSafeUint(object, "policySchema")
	revision, revisionErr := requiredJSONSafeUint(object, "policyRev")
	prior, priorErr := requiredJSONString(object, "priorPolicySha256")
	state, stateErr := requiredJSONString(object, "state")
	if schemaErr != nil || revisionErr != nil || priorErr != nil || stateErr != nil || schema != 1 || revision == 0 || (revision == 1 && prior != "") || (revision > 1 && !lowercaseProjectionSHA(prior)) {
		return 0, "", errors.New("invalid admission policy")
	}
	_, hasActive := object["maxActiveRuns"]
	_, hasQueued := object["maxQueuedRuns"]
	switch state {
	case "disabled":
		if hasActive || hasQueued {
			return 0, "", errors.New("disabled policy has limits")
		}
	case "configured":
		active, activeErr := requiredJSONSafeUint(object, "maxActiveRuns")
		queued, queuedErr := requiredJSONSafeUint(object, "maxQueuedRuns")
		if activeErr != nil || queuedErr != nil || active == 0 || queued == 0 || active > runtimeAuthorityMaxRunLimit || queued > runtimeAuthorityMaxRunLimit {
			return 0, "", errors.New("invalid configured policy limits")
		}
	default:
		return 0, "", errors.New("unknown admission policy state")
	}
	return revision, prior, nil
}

func (state *projectionState) finishRun(status, disposition string, sequence uint64) {
	state.view.Status, state.view.Final, state.terminal = status, true, true
	for index := range state.view.Attempts {
		attempt := &state.view.Attempts[index]
		if attempt.CompletedSeq != 0 {
			continue
		}
		attempt.Status, attempt.Disposition, attempt.CompletedSeq = status, disposition, sequence
		if node := state.node(attempt.NodeID); node != nil {
			node.Status, node.FinalDisposition = status, disposition
		}
	}
}

func parseSchema2Bindings(raw []byte) (schema2BindingsSnapshot, error) {
	result := schema2BindingsSnapshot{Bindings: map[string]schema2Binding{}}
	var current *schema2Binding
	currentKeys := map[string]bool{}
	topKeys := map[string]bool{}
	flush := func() error {
		if current == nil {
			return nil
		}
		if !safeProjectionIdentifier(current.BindingID) || !safeProjectionIdentifier(current.NodeID) ||
			!safeProjectionIdentifier(current.SlotID) || !safeProjectionIdentifier(current.AgentID) ||
			!safeProjectionIdentifier(current.Harness) || !safeProjectionIdentifier(current.SessionTargetID) ||
			!lowercaseProjectionSHA(current.TargetFingerprint) || !lowercaseProjectionSHA(current.SessionLineageSHA256) || len(currentKeys) != 8 {
			return projectionError(ErrRunProjectionInvalid, "invalid schema-2 binding")
		}
		if _, exists := result.Bindings[current.BindingID]; exists {
			return projectionError(ErrRunProjectionInvalid, "duplicate schema-2 binding")
		}
		result.Bindings[current.BindingID] = *current
		return nil
	}
	for _, line := range splitLines(raw) {
		trimmed := strings.TrimSpace(line.body)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || line.valueContinuation {
			continue
		}
		section, isSection := tomlLineSectionName(line)
		if isSection {
			if err := flush(); err != nil {
				return schema2BindingsSnapshot{}, err
			}
			current = nil
			if section != "binding" || !strings.HasPrefix(trimmed, "[[") {
				return schema2BindingsSnapshot{}, projectionError(ErrRunProjectionInvalid, "unknown schema-2 bindings section")
			}
			current = &schema2Binding{}
			currentKeys = map[string]bool{}
			continue
		}
		if isTOMLHeader(line) {
			return schema2BindingsSnapshot{}, projectionError(ErrRunProjectionInvalid, "invalid schema-2 bindings section")
		}
		key, value, ok := tomlKeyValue(line.body)
		if !ok {
			return schema2BindingsSnapshot{}, projectionError(ErrRunProjectionInvalid, "malformed schema-2 bindings line")
		}
		if current == nil {
			if topKeys[key] || !stringSet("schema", "runId", "boardId", "boardRev")[key] {
				return schema2BindingsSnapshot{}, projectionError(ErrRunProjectionInvalid, "invalid schema-2 bindings metadata")
			}
			topKeys[key] = true
			switch key {
			case "schema":
				result.Schema, _ = strconv.ParseUint(value, 10, 64)
			case "runId":
				result.RunID = parseString(value)
			case "boardId":
				result.BoardID = parseString(value)
			case "boardRev":
				result.BoardRev, _ = strconv.ParseUint(value, 10, 64)
			}
			continue
		}
		if currentKeys[key] || !stringSet("bindingId", "nodeId", "slotId", "agentId", "harness", "sessionTargetId", "targetFingerprint", "sessionLineageSha256")[key] {
			return schema2BindingsSnapshot{}, projectionError(ErrRunProjectionInvalid, "invalid schema-2 binding member")
		}
		currentKeys[key] = true
		switch key {
		case "bindingId":
			current.BindingID = parseString(value)
		case "nodeId":
			current.NodeID = parseString(value)
		case "slotId":
			current.SlotID = parseString(value)
		case "agentId":
			current.AgentID = parseString(value)
		case "harness":
			current.Harness = parseString(value)
		case "sessionTargetId":
			current.SessionTargetID = parseString(value)
		case "targetFingerprint":
			current.TargetFingerprint = parseString(value)
		case "sessionLineageSha256":
			current.SessionLineageSHA256 = parseString(value)
		}
	}
	if err := flush(); err != nil {
		return schema2BindingsSnapshot{}, err
	}
	if len(topKeys) != 4 || result.Schema != 2 || !validRunID(result.RunID) || !safeProjectionIdentifier(result.BoardID) || result.BoardRev == 0 || result.BoardRev > MaxJSONSafeInteger {
		return schema2BindingsSnapshot{}, projectionError(ErrRunProjectionInvalid, "invalid schema-2 bindings metadata")
	}
	return result, nil
}

func validateSchema2Documents(runID string, documents canonicalDocuments, events []rawProjectionEvent, started SafeSchema2RunStartedEvent) (schema2AuthorityProjection, map[string]RunCommandReceipt, error) {
	registry, err := validateProjectionWorkspaceRegistry(oneDocument(documents, CanonicalInputRoleSchema2WorkspaceRegistry).Bytes)
	if err != nil {
		return schema2AuthorityProjection{}, nil, err
	}
	bootstrap, err := validateProjectionWorkspaceBootstrap(oneDocument(documents, CanonicalInputRoleSchema2WorkspaceBootstrap).Bytes)
	if err != nil {
		return schema2AuthorityProjection{}, nil, err
	}
	authority, err := validateProjectionWorkspaceAuthority(oneDocument(documents, CanonicalInputRoleSchema2WorkspaceAuthority).Bytes)
	if err != nil {
		return schema2AuthorityProjection{}, nil, err
	}
	entry, ok := registry[authority.WorkspaceAuthorityID]
	if !ok || bootstrap.WorkspaceAuthorityID != authority.WorkspaceAuthorityID || bootstrap.WorkspaceRootIdentitySHA256 != authority.WorkspaceRootIdentitySHA256 || entry != authority.WorkspaceRootIdentitySHA256 || started.Data.WorkspaceAuthorityID != authority.WorkspaceAuthorityID || started.Data.WorkspaceAdmissionSeq == 0 || started.Data.WorkspaceAdmissionSeq >= authority.NextAdmissionSeq {
		return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "workspace authority identity mismatch")
	}
	runBootstrap, err := validateProjectionRunBootstrap(oneDocument(documents, CanonicalInputRoleSchema2RunBootstrap).Bytes)
	if err != nil || runBootstrap.WorkspaceAuthorityID != authority.WorkspaceAuthorityID || runBootstrap.RunID != runID {
		return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "invalid run bootstrap")
	}

	type policyRecord struct {
		Document CanonicalInputDocument
		Prior    string
	}
	policies := make(map[uint64]policyRecord)
	for _, document := range documents.byRole[CanonicalInputRoleSchema2AdmissionPolicy] {
		policyRev, prior, policyErr := validateProjectionAdmissionPolicy(document.Bytes)
		if policyErr != nil || policyRev > authority.AdmissionPolicyRef.PolicyRev {
			return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "invalid admission policy")
		}
		if _, exists := policies[policyRev]; exists {
			return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "duplicate admission policy")
		}
		policies[policyRev] = policyRecord{Document: document, Prior: prior}
	}
	for revision := uint64(1); revision <= authority.AdmissionPolicyRef.PolicyRev; revision++ {
		policy, present := policies[revision]
		if !present || (revision == 1 && policy.Prior != "") || (revision > 1 && policy.Prior != policies[revision-1].Document.SHA256) {
			return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "broken admission policy chain")
		}
	}
	selected, ok := policies[authority.AdmissionPolicyRef.PolicyRev]
	if !ok || len(policies) != int(authority.AdmissionPolicyRef.PolicyRev) || selected.Document.SHA256 != authority.AdmissionPolicyRef.PolicySHA256 {
		return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "incomplete admission policy chain")
	}

	referenced := map[string]struct {
		payload string
		seq     uint64
	}{}
	for _, event := range events {
		var commandID, payload string
		switch event.typeName {
		case "run_started":
			commandID, _ = requiredJSONString(event.data, "admissionCommandId")
			payload, _ = requiredJSONString(event.data, "commandPayloadSha256")
		case "run_resumed", "human_verdict_recorded", "run_cancel_requested":
			commandID, _ = requiredJSONString(event.data, "commandId")
			payload, _ = requiredJSONString(event.data, "commandPayloadSha256")
		}
		if commandID != "" {
			if previous, exists := referenced[commandID]; exists && (previous.payload != payload || previous.seq != event.envelope.Seq) {
				return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "command referenced inconsistently")
			}
			referenced[commandID] = struct {
				payload string
				seq     uint64
			}{payload: payload, seq: event.envelope.Seq}
		}
	}
	commands := make(map[string]RunCommandReceipt)
	for _, document := range documents.byRole[CanonicalInputRoleSchema2CommandRecord] {
		object, err := decodeUniqueJSONObject(document.Bytes)
		if err != nil {
			return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "invalid command record")
		}
		commandID, _ := requiredJSONString(object, "commandId")
		commandKind, _ := requiredJSONString(object, "commandKind")
		payload, _ := requiredJSONString(object, "commandPayloadSha256")
		if _, exists := commands[commandID]; exists {
			return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "duplicate command record")
		}
		if _, exists := referenced[commandID]; !exists {
			return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "unreferenced command record")
		}
		receipt, receiptErr := ProjectCommandReceipt(CanonicalCommandReadInput{
			Source:    CanonicalRunSourceSchema2,
			Submitted: SubmittedCommandIdentity{CommandID: commandID, CommandKind: commandKind, CommandPayloadSHA256: payload},
			Record:    document.Bytes,
		})
		if receiptErr != nil {
			return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "invalid command receipt")
		}
		commands[commandID] = receipt
	}
	if len(commands) != len(referenced) {
		return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "incomplete command set")
	}
	if receipt, ok := commands[events[0].dataString("admissionCommandId")].(RunCommandAppliedReceipt); !ok || receipt.RunID != runID || receipt.DecisionAdmissionPolicyRef == nil || *receipt.DecisionAdmissionPolicyRef != authority.AdmissionPolicyRef {
		return schema2AuthorityProjection{}, nil, projectionError(ErrRunProjectionInvalid, "admission command mismatch")
	}
	return authority, commands, nil
}

func (event rawProjectionEvent) dataString(key string) string {
	value, _ := requiredJSONString(event.data, key)
	return value
}

func lowercaseProjectionSHA(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func verifyEventCommand(commandID, payloadSHA string, sequence uint64, commands map[string]RunCommandReceipt) error {
	receipt, ok := commands[commandID].(RunCommandAppliedReceipt)
	if !ok || receipt.CommandPayloadSHA256 != payloadSHA || receipt.EffectSeq != sequence {
		return projectionError(ErrRunProjectionInvalid, "event command linkage mismatch")
	}
	return nil
}

func (state *projectionState) addSchema2Dispatch(raw rawProjectionEvent, dispatch SafeSchema2SlotDispatchData) error {
	binding, ok := state.bindings[dispatch.BindingID]
	if !ok || binding.NodeID != dispatch.NodeID || binding.SlotID != dispatch.SlotID || binding.AgentID != dispatch.AgentID ||
		binding.Harness != dispatch.Harness || binding.SessionTargetID != dispatch.SessionTargetID || binding.TargetFingerprint != dispatch.TargetFingerprint || !dispatch.RecordedBeforeSend {
		return projectionError(ErrRunProjectionInvalid, "dispatch binding mismatch")
	}
	if _, exists := state.dispatches[dispatch.DispatchID]; exists {
		return projectionError(ErrRunProjectionInvalid, "duplicate dispatch")
	}
	state.dispatches[dispatch.DispatchID] = dispatch
	state.dispatchSeq[dispatch.DispatchID] = raw.envelope.Seq
	health, ok := state.health[dispatch.BindingID]
	if !ok || health.SessionTargetID != dispatch.SessionTargetID || health.SlotID != dispatch.SlotID {
		return projectionError(ErrRunProjectionInvalid, "dispatch without runnable binding observation")
	}
	state.ensureAttempt(dispatch.NodeID, dispatch.Attempt)
	state.view.Sessions = append(state.view.Sessions, RunSessionView{
		BindingID: dispatch.BindingID, NodeID: dispatch.NodeID, Attempt: dispatch.Attempt, SlotID: dispatch.SlotID,
		DispatchID: dispatch.DispatchID, TargetLeaseID: dispatch.TargetLeaseID, SessionTargetID: dispatch.SessionTargetID,
		BindingHealth: health.Health, SessionLineageSHA256: binding.SessionLineageSHA256,
		TargetFingerprintSHA256: projectionSHA([]byte(dispatch.TargetFingerprint)),
		Baseline:                RunSessionBaseline{Encoding: dispatch.PaneHistoryBaselineEncoding, SHA256: dispatch.PaneHistoryBaselineSHA256, State: "valid"},
		Attachment:              RunSessionAttachment{State: "accounted"}, Occupancy: RunSessionOccupancy{State: "active"},
		PeekCapability: RunSessionPeekCapability{State: "none", Generation: "0"},
		Steering:       RunSessionSteering{State: "closed", Generation: dispatch.SteeringGeneration},
	})
	return nil
}

func (state *projectionState) sessionByDispatch(dispatchID string) *RunSessionView {
	for index := range state.view.Sessions {
		if state.view.Sessions[index].DispatchID == dispatchID {
			return &state.view.Sessions[index]
		}
	}
	return nil
}

func (state *projectionState) setSessionCapability(dispatchID, capabilityState string, issuedSeq uint64, generation string) {
	if session := state.sessionByDispatch(dispatchID); session != nil {
		session.PeekCapability = RunSessionPeekCapability{State: capabilityState, IssuedSeq: issuedSeq, Generation: generation}
	}
}

func (state *projectionState) setSessionOperatorInfluenced(dispatchID string, influenced bool) {
	if session := state.sessionByDispatch(dispatchID); session != nil {
		session.OperatorInfluenced = influenced
	}
}

func (state *projectionState) currentSchema2OpenDispatches() []SafeSchema2OpenDispatch {
	result := make([]SafeSchema2OpenDispatch, 0, len(state.dispatches))
	for dispatchID, dispatch := range state.dispatches {
		if state.matchedDispatch[dispatchID] {
			continue
		}
		item := SafeSchema2OpenDispatch{
			DispatchID: dispatch.DispatchID, TargetLeaseID: dispatch.TargetLeaseID, NodeID: dispatch.NodeID, Attempt: dispatch.Attempt,
			SlotID: dispatch.SlotID, AgentID: dispatch.AgentID, BindingID: dispatch.BindingID, SessionTargetID: dispatch.SessionTargetID,
			TargetFingerprint: dispatch.TargetFingerprint, DispatchSeq: state.dispatchSeq[dispatchID], PeekCapabilityState: "none", LatestCapabilityGeneration: "0",
			LatestCapabilityIssuedSeq: 0, LatestSteeringGeneration: dispatch.SteeringGeneration, InterruptState: "none",
		}
		for _, event := range state.view.Sessions {
			if event.DispatchID == dispatchID {
				item.PeekCapabilityState = event.PeekCapability.State
				item.LatestCapabilityGeneration = event.PeekCapability.Generation
				item.LatestCapabilityIssuedSeq = event.PeekCapability.IssuedSeq
				item.LatestSteeringGeneration = event.Steering.Generation
			}
		}
		if sequence, revoked := state.revokedDispatch[dispatchID]; revoked {
			value := sequence
			item.PeekCapabilityRevokedSeq = &value
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].DispatchSeq < result[j].DispatchSeq })
	return result
}

func (state *projectionState) validateSchema2OpenDispatches(got []SafeSchema2OpenDispatch) error {
	want := state.currentSchema2OpenDispatches()
	for index := range want {
		want[index].DispatchSeq = state.dispatchSequence(want[index].DispatchID)
	}
	sort.Slice(want, func(i, j int) bool { return want[i].DispatchSeq < want[j].DispatchSeq })
	left, _ := json.Marshal(got)
	right, _ := json.Marshal(want)
	if !bytes.Equal(left, right) {
		return projectionError(ErrRunProjectionInvalid, "open dispatch carry mismatch")
	}
	return nil
}

func (state *projectionState) dispatchSequence(dispatchID string) uint64 {
	return state.dispatchSeq[dispatchID]
}

func (state *projectionState) appendSchema2Block(raw rawProjectionEvent, data SafeSchema2RunBlockedData) {
	dispatches := make([]SafeOpenDispatch, len(data.OpenDispatches))
	for index := range data.OpenDispatches {
		dispatches[index] = data.OpenDispatches[index]
	}
	state.view.Blocks = append(state.view.Blocks, RunBlockView{
		Seq: raw.envelope.Seq, Epoch: raw.envelope.Epoch, Scope: data.BlockScope, NodeID: data.BlockedNodeID,
		GateID: data.BlockedGateID, Reason: data.Reason, ResumeAllowed: data.ResumeAllowed,
		ResumePolicy: data.ResumePolicy, NextEpoch: data.NextEpoch, OpenDispatches: dispatches,
	})
	state.lastBlockJSON, _ = json.Marshal(data.OpenDispatches)
}

func artifactProjectionID(artifact ArtifactProjection) string {
	switch value := artifact.(type) {
	case AvailableArtifactProjection:
		return value.ArtifactID
	case UnavailableArtifactProjection:
		return value.ArtifactID
	default:
		return ""
	}
}

func artifactProjectionName(artifact ArtifactProjection) string {
	switch value := artifact.(type) {
	case AvailableArtifactProjection:
		return value.Name
	case UnavailableArtifactProjection:
		return value.Name
	default:
		return ""
	}
}

func artifactByID(artifacts []ArtifactProjection, artifactID string) (ArtifactProjection, bool) {
	for _, artifact := range artifacts {
		if artifactProjectionID(artifact) == artifactID {
			return artifact, true
		}
	}
	return nil, false
}

func (state *projectionState) upsertArtifact(artifact ArtifactProjection) {
	artifactID := artifactProjectionID(artifact)
	if artifactID == "" {
		return
	}
	if index, ok := state.artifactIndex[artifactID]; ok {
		state.view.Artifacts[index] = artifact
		return
	}
	state.artifactIndex[artifactID] = len(state.view.Artifacts)
	state.view.Artifacts = append(state.view.Artifacts, artifact)
}

func (state *projectionState) observeArtifact(observed SafeSchema2ArtifactObservedData) {
	index, ok := state.artifactIndex[observed.ArtifactID]
	if !ok {
		return
	}
	name := artifactProjectionName(state.view.Artifacts[index])
	if observed.Availability == "available" && observed.Artifact != nil {
		state.view.Artifacts[index] = AvailableArtifactProjection{ArtifactID: observed.ArtifactID, Availability: "available", Name: name, Artifact: *observed.Artifact}
		return
	}
	state.view.Artifacts[index] = UnavailableArtifactProjection{ArtifactID: observed.ArtifactID, Availability: observed.Availability, Name: name, ErrorCode: observed.ErrorCode}
}

func finalizeProjectionState(state *projectionState) {
	order := func(nodeID string) int {
		if index, ok := state.nodeIndex[nodeID]; ok {
			return index
		}
		return len(state.nodeIndex)
	}
	sort.SliceStable(state.view.Attempts, func(i, j int) bool {
		left, right := state.view.Attempts[i], state.view.Attempts[j]
		if order(left.NodeID) != order(right.NodeID) {
			return order(left.NodeID) < order(right.NodeID)
		}
		return left.Attempt < right.Attempt
	})
	sort.SliceStable(state.view.Gates, func(i, j int) bool {
		left, right := state.view.Gates[i], state.view.Gates[j]
		if order(left.GateID) != order(right.GateID) {
			return order(left.GateID) < order(right.GateID)
		}
		return left.Attempt < right.Attempt
	})
	sort.SliceStable(state.view.Outputs, func(i, j int) bool {
		left, right := state.view.Outputs[i], state.view.Outputs[j]
		if order(left.NodeID) != order(right.NodeID) {
			return order(left.NodeID) < order(right.NodeID)
		}
		if left.Attempt != right.Attempt {
			return left.Attempt < right.Attempt
		}
		ports := state.outputPortIDs(left.NodeID)
		portOrder := func(portID string) int {
			for index, candidate := range ports {
				if candidate == portID {
					return index
				}
			}
			return len(ports)
		}
		return portOrder(left.PortID) < portOrder(right.PortID)
	})
	slotOrder := func(nodeID, slotID string) int {
		if state.board != nil {
			for _, node := range state.board.Formations {
				if node.ID == nodeID {
					for index, slot := range node.Slots {
						if slot.ID == slotID {
							return index
						}
					}
				}
			}
		}
		return 1 << 30
	}
	sort.SliceStable(state.view.Sessions, func(i, j int) bool {
		left, right := state.view.Sessions[i], state.view.Sessions[j]
		if order(left.NodeID) != order(right.NodeID) {
			return order(left.NodeID) < order(right.NodeID)
		}
		if left.Attempt != right.Attempt {
			return left.Attempt < right.Attempt
		}
		return slotOrder(left.NodeID, left.SlotID) < slotOrder(right.NodeID, right.SlotID)
	})
	for index := range state.view.Nodes {
		state.view.Nodes[index].Attempts = []RunAttemptRef{}
		state.view.Nodes[index].Outputs = []RunOutputRef{}
		state.view.Nodes[index].Gates = []RunGateRef{}
		state.view.Nodes[index].Sessions = []RunSessionRef{}
	}
	for index := range state.view.Attempts {
		attempt := &state.view.Attempts[index]
		attempt.Outputs, attempt.Slots, attempt.Gate = []RunOutputRef{}, []RunSessionRef{}, nil
		if node := state.node(attempt.NodeID); node != nil {
			node.Attempts = append(node.Attempts, RunAttemptRef{NodeID: attempt.NodeID, Attempt: attempt.Attempt})
		}
	}
	for _, output := range state.view.Outputs {
		ref := RunOutputRef{NodeID: output.NodeID, Attempt: output.Attempt, PortID: output.PortID}
		if node := state.node(output.NodeID); node != nil {
			node.Outputs = append(node.Outputs, ref)
		}
		for index := range state.view.Attempts {
			if state.view.Attempts[index].NodeID == output.NodeID && state.view.Attempts[index].Attempt == output.Attempt {
				state.view.Attempts[index].Outputs = append(state.view.Attempts[index].Outputs, ref)
			}
		}
	}
	for _, gate := range state.view.Gates {
		ref := RunGateRef{GateID: gate.GateID, Attempt: gate.Attempt}
		if node := state.node(gate.GateID); node != nil {
			node.Gates = append(node.Gates, ref)
		}
		for index := range state.view.Attempts {
			if state.view.Attempts[index].NodeID == gate.GateID && state.view.Attempts[index].Attempt == gate.Attempt {
				copyRef := ref
				state.view.Attempts[index].Gate = &copyRef
			}
		}
	}
	for _, session := range state.view.Sessions {
		ref := RunSessionRef{BindingID: session.BindingID, NodeID: session.NodeID, Attempt: session.Attempt, SlotID: session.SlotID}
		if node := state.node(session.NodeID); node != nil {
			node.Sessions = append(node.Sessions, ref)
		}
		for index := range state.view.Attempts {
			if state.view.Attempts[index].NodeID == session.NodeID && state.view.Attempts[index].Attempt == session.Attempt {
				state.view.Attempts[index].Slots = append(state.view.Attempts[index].Slots, ref)
			}
		}
	}
	if state.view.Source.EventSchema == 1 && state.view.Status == RunStatusSucceeded {
		for _, node := range state.view.Nodes {
			if node.Status == "waiting" {
				state.view.Status = RunStatusBlocked
				state.view.Final = false
				break
			}
		}
	}
}
