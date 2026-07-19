package formations

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

const (
	testAuthorityRecordWorkspaceID  = "wsa_01KXNP6VY3227H78329V52CKF8"
	testAuthorityRecordWorkspaceID2 = "wsa_01KXNP6VY3227H78329V52CKF9"
	testAuthorityRecordWorkspaceID3 = "wsa_01KXNP6VY3227H78329V52CKFA"
	testAuthorityRecordCommandID    = "cmd_01KXNP6VY3227H78329V52CKF8"
	testAuthorityRecordRunID        = "run_01KXNP6VY3227H78329V52CKF8"
)

type testAuthorityCodecResult struct {
	recordRev       uint64
	priorGeneration *authorityGeneration
	encoded         []byte
}

type testAuthorityCodec struct {
	name      string
	recordRaw func(uint64, *authorityGeneration) []byte
	roundTrip func([]byte) (testAuthorityCodecResult, error)
}

func TestAuthorityMutableRecordCodecsRoundTripOnlyExactNamedEncoding(t *testing.T) {
	for _, codec := range testAuthorityRecordCodecs() {
		t.Run(codec.name, func(t *testing.T) {
			firstRaw := codec.recordRaw(1, nil)
			first, err := codec.roundTrip(firstRaw)
			if err != nil {
				t.Fatalf("decode revision 1: %v", err)
			}
			if first.recordRev != 1 || first.priorGeneration != nil {
				t.Fatalf("revision 1 metadata = rev %d, prior %+v; want rev 1 with explicit null predecessor", first.recordRev, first.priorGeneration)
			}
			if !bytes.Equal(first.encoded, firstRaw) {
				t.Fatalf("revision 1 round trip changed frozen %s bytes\n got: %s\nwant: %s", codec.name, first.encoded, firstRaw)
			}
			if err := validateAuthorityRecordTransition(first.recordRev, first.priorGeneration, nil); err != nil {
				t.Fatalf("initial transition with explicit null predecessor: %v", err)
			}

			predecessor := authorityGeneration{recordRev: 1, sha256: runtimeSHA256Hex(firstRaw)}
			secondRaw := codec.recordRaw(2, &predecessor)
			second, err := codec.roundTrip(secondRaw)
			if err != nil {
				t.Fatalf("decode revision 2: %v", err)
			}
			if second.recordRev != 2 || second.priorGeneration == nil || *second.priorGeneration != predecessor {
				t.Fatalf("revision 2 metadata = rev %d, prior %+v; want authenticated predecessor %+v", second.recordRev, second.priorGeneration, predecessor)
			}
			if !bytes.Equal(second.encoded, secondRaw) {
				t.Fatalf("revision 2 round trip changed frozen %s bytes\n got: %s\nwant: %s", codec.name, second.encoded, secondRaw)
			}
			if err := validateAuthorityRecordTransition(second.recordRev, second.priorGeneration, &predecessor); err != nil {
				t.Fatalf("exact predecessor transition: %v", err)
			}
		})
	}
}

func TestAuthorityMutableRecordCodecsRejectUnauditableBytes(t *testing.T) {
	for _, codec := range testAuthorityRecordCodecs() {
		t.Run(codec.name, func(t *testing.T) {
			firstRaw := codec.recordRaw(1, nil)
			predecessor := authorityGeneration{recordRev: 1, sha256: runtimeSHA256Hex(firstRaw)}
			secondRaw := codec.recordRaw(2, &predecessor)

			wrongRevision := predecessor
			wrongRevision.recordRev = 2
			wrongHash := predecessor
			wrongHash.sha256 = strings.Repeat("0", 64)

			tests := []struct {
				name string
				raw  []byte
			}{
				{name: "missing explicit revision one predecessor", raw: testAuthorityRemovePriorGeneration(firstRaw, "null")},
				{name: "revision one has predecessor", raw: codec.recordRaw(1, &predecessor)},
				{name: "revision two missing predecessor", raw: testAuthorityRemovePriorGeneration(secondRaw, testAuthorityPriorGenerationJSON(&predecessor))},
				{name: "revision two names wrong predecessor revision", raw: codec.recordRaw(2, &wrongRevision)},
				{name: "adjacent duplicate key", raw: testAuthorityReplaceOnce(firstRaw, []byte(`"recordRev":1`), []byte(`"recordRev":1,"recordRev":1`))},
				{name: "unknown key in canonical position", raw: testAuthorityAppendObjectField(firstRaw, `"zzUnknown":true`)},
				{name: "leading whitespace", raw: append([]byte(" "), firstRaw...)},
				{name: "noncanonical key order", raw: testAuthorityMoveRecordRevFirst(firstRaw)},
				{name: "utf8 bom", raw: append([]byte{0xef, 0xbb, 0xbf}, firstRaw...)},
				{name: "trailing newline", raw: append(append([]byte(nil), firstRaw...), '\n')},
				{name: "unsafe json integer", raw: bytes.Replace(firstRaw, []byte(`"recordRev":1`), []byte(`"recordRev":9007199254740992`), 1)},
			}

			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					if _, err := codec.roundTrip(test.raw); err == nil {
						t.Fatalf("accepted non-authorizing %s bytes: %s", codec.name, test.raw)
					}
				})
			}

			second, err := codec.roundTrip(secondRaw)
			if err != nil {
				t.Fatalf("decode canonical revision 2 for transition checks: %v", err)
			}
			wrongRecord, err := codec.roundTrip(codec.recordRaw(2, &wrongHash))
			if err != nil {
				t.Fatalf("decode structurally canonical revision 2 before predecessor authentication: %v", err)
			}
			if err := validateAuthorityRecordTransition(wrongRecord.recordRev, wrongRecord.priorGeneration, &predecessor); err == nil {
				t.Fatalf("accepted record with wrong predecessor hash: record %+v, expected %+v", wrongRecord.priorGeneration, predecessor)
			}
			for _, test := range []struct {
				name     string
				expected *authorityGeneration
			}{
				{name: "missing caller predecessor", expected: nil},
				{name: "wrong caller predecessor revision", expected: &wrongRevision},
				{name: "wrong caller predecessor hash", expected: &wrongHash},
			} {
				t.Run(test.name, func(t *testing.T) {
					if err := validateAuthorityRecordTransition(second.recordRev, second.priorGeneration, test.expected); err == nil {
						t.Fatalf("accepted unauthenticated predecessor: record %+v, expected %+v", second.priorGeneration, test.expected)
					}
				})
			}
		})
	}
}

func TestAuthorityMutableRecordCodecsRejectNestedClosedShapeViolations(t *testing.T) {
	registryEntry := testAuthorityRegistryEntryRaw("/srv/work", "1", "2", testAuthorityRecordWorkspaceID, strings.Repeat("a", 64))
	registryRaw := testAuthorityRegistryRecordRawWithEntries(1, nil, registryEntry)
	policyRef := []byte(testAuthorityAdmissionPolicyRefJSON())
	workspaceRaw := testAuthorityWorkspaceRecordRaw(1, nil)
	firstRaw := testAuthorityRegistryRecordRaw(1, nil)
	predecessor := authorityGeneration{recordRev: 1, sha256: runtimeSHA256Hex(firstRaw)}
	prior := []byte(testAuthorityPriorGenerationJSON(&predecessor))
	secondRaw := testAuthorityRegistryRecordRaw(2, &predecessor)
	cancelPayload := testAuthorityCommandPayloadByKind("cancel")

	tests := []struct {
		name      string
		codecName string
		raw       []byte
	}{
		{name: "registry entry unknown key", codecName: "workspace-registry-jcs-v1", raw: testAuthorityReplaceOnce(registryRaw, registryEntry, testAuthorityAppendObjectField(registryEntry, `"zzUnknown":true`))},
		{name: "registry entry adjacent duplicate", codecName: "workspace-registry-jcs-v1", raw: testAuthorityReplaceOnce(registryRaw, registryEntry, testAuthorityReplaceOnce(registryEntry, []byte(`"device":"1"`), []byte(`"device":"1","device":"1"`)))},
		{name: "registry entry key order", codecName: "workspace-registry-jcs-v1", raw: testAuthorityReplaceOnce(registryRaw, registryEntry, testAuthorityMoveFieldFirst(registryEntry, `,"device":"1"`))},
		{name: "admission policy ref unknown key", codecName: "workspace-authority-jcs-v1", raw: testAuthorityReplaceOnce(workspaceRaw, policyRef, testAuthorityAppendObjectField(policyRef, `"zzUnknown":true`))},
		{name: "admission policy ref adjacent duplicate", codecName: "workspace-authority-jcs-v1", raw: testAuthorityReplaceOnce(workspaceRaw, policyRef, testAuthorityReplaceOnce(policyRef, []byte(`"policyRev":1`), []byte(`"policyRev":1,"policyRev":1`)))},
		{name: "admission policy ref key order", codecName: "workspace-authority-jcs-v1", raw: testAuthorityReplaceOnce(workspaceRaw, policyRef, testAuthorityMoveFieldFirst(policyRef, `,"policySha256":"`+strings.Repeat("b", 64)+`"`))},
		{name: "prior generation unknown key", codecName: "workspace-registry-jcs-v1", raw: testAuthorityReplaceOnce(secondRaw, prior, testAuthorityAppendObjectField(prior, `"zzUnknown":true`))},
		{name: "prior generation adjacent duplicate", codecName: "workspace-registry-jcs-v1", raw: testAuthorityReplaceOnce(secondRaw, prior, testAuthorityReplaceOnce(prior, []byte(`"recordRev":1`), []byte(`"recordRev":1,"recordRev":1`)))},
		{name: "prior generation key order", codecName: "workspace-registry-jcs-v1", raw: testAuthorityReplaceOnce(secondRaw, prior, testAuthorityMoveFieldFirst(prior, `,"sha256":"`+predecessor.sha256+`"`))},
		{name: "command payload unknown key", codecName: "run-command-record-jcs-v1", raw: testAuthorityCommandRecordRawFor(1, nil, "cancel", testAuthorityAppendObjectField(cancelPayload, `"zzUnknown":true`), "pending")},
		{name: "command payload adjacent duplicate", codecName: "run-command-record-jcs-v1", raw: testAuthorityCommandRecordRawFor(1, nil, "cancel", testAuthorityReplaceOnce(cancelPayload, []byte(`"actor":"agent:test"`), []byte(`"actor":"agent:test","actor":"agent:test"`)), "pending")},
		{name: "command payload key order", codecName: "run-command-record-jcs-v1", raw: testAuthorityCommandRecordRawFor(1, nil, "cancel", testAuthorityMoveFieldFirst(cancelPayload, `,"authoritySchema":2`), "pending")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := testAuthorityCodecByName(test.codecName).roundTrip(test.raw); err == nil {
				t.Fatalf("accepted nested non-authorizing bytes: %s", test.raw)
			}
		})
	}
}

func TestWorkspaceRegistryCodecUsesCanonicalNumericIdentityOrder(t *testing.T) {
	entries := [][]byte{
		testAuthorityRegistryEntryRaw("/srv/z", "2", "10", testAuthorityRecordWorkspaceID, strings.Repeat("a", 64)),
		testAuthorityRegistryEntryRaw("/srv/y", "10", "1", testAuthorityRecordWorkspaceID2, strings.Repeat("b", 64)),
		testAuthorityRegistryEntryRaw("/srv/a", "10", "2", testAuthorityRecordWorkspaceID3, strings.Repeat("c", 64)),
	}
	canonical := testAuthorityRegistryRecordRawWithEntries(1, nil, entries...)
	result, err := testAuthorityCodecByName("workspace-registry-jcs-v1").roundTrip(canonical)
	if err != nil {
		t.Fatalf("decode numerically sorted registry: %v", err)
	}
	if !bytes.Equal(result.encoded, canonical) {
		t.Fatalf("numeric registry ordering changed on round trip\n got: %s\nwant: %s", result.encoded, canonical)
	}

	tests := []struct {
		name    string
		entries [][]byte
	}{
		{name: "device order is numeric not lexical", entries: [][]byte{entries[1], entries[0], entries[2]}},
		{name: "inode order is numeric", entries: [][]byte{entries[0], entries[2], entries[1]}},
		{name: "duplicate configured path", entries: [][]byte{entries[0], testAuthorityRegistryEntryRaw("/srv/z", "20", "1", testAuthorityRecordWorkspaceID2, strings.Repeat("b", 64))}},
		{name: "duplicate opened identity", entries: [][]byte{testAuthorityRegistryEntryRaw("/srv/x", "2", "10", testAuthorityRecordWorkspaceID2, strings.Repeat("b", 64)), entries[0]}},
		{name: "duplicate authority id", entries: [][]byte{entries[0], testAuthorityRegistryEntryRaw("/srv/x", "20", "1", testAuthorityRecordWorkspaceID, strings.Repeat("b", 64))}},
		{name: "duplicate root identity hash", entries: [][]byte{entries[0], testAuthorityRegistryEntryRaw("/srv/x", "20", "1", testAuthorityRecordWorkspaceID2, strings.Repeat("a", 64))}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := testAuthorityRegistryRecordRawWithEntries(1, nil, test.entries...)
			if _, err := testAuthorityCodecByName("workspace-registry-jcs-v1").roundTrip(raw); err == nil {
				t.Fatalf("accepted conflicting or unsorted registry: %s", raw)
			}
		})
	}
}

func TestWorkspaceOwnerLeaseCodecRejectsNonAuthorizingLeaseEvidence(t *testing.T) {
	valid := testAuthorityOwnerLeaseRecordRaw(1, nil)
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "wrong lease schema", raw: testAuthorityReplaceOnce(valid, []byte(`"leaseSchema":1`), []byte(`"leaseSchema":2`))},
		{name: "zero writer fence", raw: testAuthorityReplaceOnce(valid, []byte(`"writerFence":1`), []byte(`"writerFence":0`))},
		{name: "unsafe writer fence", raw: testAuthorityReplaceOnce(valid, []byte(`"writerFence":1`), []byte(`"writerFence":9007199254740992`))},
		{name: "noncanonical fractional timestamp", raw: testAuthorityReplaceOnce(valid, []byte(`2026-07-19T00:00:00Z`), []byte(`2026-07-19T00:00:00.000Z`))},
		{name: "non utc z timestamp", raw: testAuthorityReplaceOnce(valid, []byte(`2026-07-19T00:00:00Z`), []byte(`2026-07-19T00:00:00+00:00`))},
		{name: "acquired after renewed", raw: testAuthorityReplaceOnce(valid, []byte(`"acquiredAt":"2026-07-19T00:00:00Z"`), []byte(`"acquiredAt":"2026-07-19T00:00:02Z"`))},
		{name: "renewed at lease end", raw: testAuthorityReplaceOnce(valid, []byte(`"renewedAt":"2026-07-19T00:00:01Z"`), []byte(`"renewedAt":"2026-07-19T00:00:30Z"`))},
		{name: "lease ends before renewal", raw: testAuthorityReplaceOnce(valid, []byte(`"leaseUntil":"2026-07-19T00:00:30Z"`), []byte(`"leaseUntil":"2026-07-19T00:00:00Z"`))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := testAuthorityCodecByName("workspace-owner-lease-jcs-v1").roundTrip(test.raw); err == nil {
				t.Fatalf("accepted non-authorizing lease evidence: %s", test.raw)
			}
		})
	}
}

func TestRunCommandRecordCodecAcceptsEveryClosedStateAndPayloadVariant(t *testing.T) {
	for _, payload := range testAuthorityCommandPayloads() {
		for _, state := range []string{"pending", "applied", "rejected"} {
			t.Run(payload.kind+" "+state, func(t *testing.T) {
				recordRev := uint64(1)
				var predecessor *authorityGeneration
				if state != "pending" {
					pending := testAuthorityCommandRecordRawFor(1, nil, payload.kind, payload.raw, "pending")
					prior := authorityGeneration{recordRev: 1, sha256: runtimeSHA256Hex(pending)}
					predecessor = &prior
					recordRev = 2
				}
				raw := testAuthorityCommandRecordRawFor(recordRev, predecessor, payload.kind, payload.raw, state)
				result, err := testAuthorityCommandCodecRoundTrip(raw)
				if err != nil {
					t.Fatalf("decode %s %s record: %v", payload.kind, state, err)
				}
				if !bytes.Equal(result.encoded, raw) {
					t.Fatalf("%s %s round trip changed bytes\n got: %s\nwant: %s", payload.kind, state, result.encoded, raw)
				}
			})
		}
	}
}

func TestRunCommandRecordCodecRejectsStatePayloadAndHashContradictions(t *testing.T) {
	cancelPayload := testAuthorityCommandPayloadByKind("cancel")
	pending := testAuthorityCommandRecordRawFor(1, nil, "cancel", cancelPayload, "pending")
	prior := authorityGeneration{recordRev: 1, sha256: runtimeSHA256Hex(pending)}
	applied := testAuthorityCommandRecordRawFor(2, &prior, "cancel", cancelPayload, "applied")
	rejected := testAuthorityCommandRecordRawFor(2, &prior, "cancel", cancelPayload, "rejected")
	startPayload := testAuthorityCommandPayloadByKind("start")
	startPending := testAuthorityCommandRecordRawFor(1, nil, "start", startPayload, "pending")
	startPrior := authorityGeneration{recordRev: 1, sha256: runtimeSHA256Hex(startPending)}
	startApplied := testAuthorityCommandRecordRawFor(2, &startPrior, "start", startPayload, "applied")
	startRejected := testAuthorityCommandRecordRawFor(2, &startPrior, "start", startPayload, "rejected")
	payloadHash := runtimeSHA256Hex(cancelPayload)

	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "missing state", raw: testAuthorityRemoveField(pending, `,"state":"pending"`)},
		{name: "unknown state", raw: testAuthorityReplaceOnce(pending, []byte(`"state":"pending"`), []byte(`"state":"completed"`))},
		{name: "pending has decision ref", raw: testAuthorityInsertBefore(pending, `,"priorGeneration":`, `,"decisionAdmissionPolicyRef":null`)},
		{name: "pending has effect sequence", raw: testAuthorityInsertBefore(pending, `,"priorGeneration":`, `,"effectSeq":1`)},
		{name: "pending has outcome fence", raw: testAuthorityInsertBefore(pending, `,"priorGeneration":`, `,"outcomeWriterFence":1`)},
		{name: "pending has run id", raw: testAuthorityInsertBefore(pending, `,"state":`, `,"runId":"`+testAuthorityRecordRunID+`"`)},
		{name: "pending has rejection code", raw: testAuthorityInsertBefore(pending, `,"state":`, `,"rejectionCode":"extra"`)},
		{name: "applied missing decision ref", raw: testAuthorityRemoveField(applied, `,"decisionAdmissionPolicyRef":null`)},
		{name: "applied missing effect sequence", raw: testAuthorityRemoveField(applied, `,"effectSeq":1`)},
		{name: "applied missing outcome fence", raw: testAuthorityRemoveField(applied, `,"outcomeWriterFence":1`)},
		{name: "applied missing run id", raw: testAuthorityReplaceOnce(applied, []byte(`,"recordRev":2,"runId":"`+testAuthorityRecordRunID+`"`), []byte(`,"recordRev":2`))},
		{name: "applied has rejection code", raw: testAuthorityReplaceOnce(applied, []byte(`,"recordRev":2,"runId":`), []byte(`,"recordRev":2,"rejectionCode":"extra","runId":`))},
		{name: "start applied missing decision generation", raw: testAuthorityReplaceOnce(startApplied, []byte(`"decisionAdmissionPolicyRef":`+testAuthorityAdmissionPolicyRefJSON()), []byte(`"decisionAdmissionPolicyRef":null`))},
		{name: "non start applied has decision generation", raw: testAuthorityReplaceOnce(applied, []byte(`"decisionAdmissionPolicyRef":null`), []byte(`"decisionAdmissionPolicyRef":`+testAuthorityAdmissionPolicyRefJSON()))},
		{name: "rejected missing decision ref", raw: testAuthorityRemoveField(rejected, `,"decisionAdmissionPolicyRef":null`)},
		{name: "rejected missing outcome fence", raw: testAuthorityRemoveField(rejected, `,"outcomeWriterFence":1`)},
		{name: "rejected missing rejection code", raw: testAuthorityRemoveField(rejected, `,"rejectionCode":"rejected"`)},
		{name: "rejected has effect sequence", raw: testAuthorityInsertBefore(rejected, `,"outcomeWriterFence":`, `,"effectSeq":1`)},
		{name: "rejected has run id", raw: testAuthorityInsertBefore(rejected, `,"state":`, `,"runId":"`+testAuthorityRecordRunID+`"`)},
		{name: "start rejected missing decision generation", raw: testAuthorityReplaceOnce(startRejected, []byte(`"decisionAdmissionPolicyRef":`+testAuthorityAdmissionPolicyRefJSON()), []byte(`"decisionAdmissionPolicyRef":null`))},
		{name: "non start rejected has decision generation", raw: testAuthorityReplaceOnce(rejected, []byte(`"decisionAdmissionPolicyRef":null`), []byte(`"decisionAdmissionPolicyRef":`+testAuthorityAdmissionPolicyRefJSON()))},
		{name: "outer kind mismatches payload", raw: testAuthorityCommandRecordRawFor(1, nil, "resume", cancelPayload, "pending")},
		{name: "payload hash mismatch", raw: testAuthorityReplaceOnce(pending, []byte(`"commandPayloadSha256":"`+payloadHash+`"`), []byte(`"commandPayloadSha256":"`+strings.Repeat("0", 64)+`"`))},
		{name: "start payload missing run root", raw: testAuthorityCommandRecordRawFor(1, nil, "start", testAuthorityRemoveField(startPayload, `,"runRoot":{"kind":"mission","nodeId":"mis_01J9_improve"}`), "pending")},
		{name: "resume payload missing resume mode", raw: testAuthorityCommandRecordRawFor(1, nil, "resume", testAuthorityRemoveField(testAuthorityCommandPayloadByKind("resume"), `,"resumeMode":"retry-failed-producer"`), "pending")},
		{name: "verdict payload missing gate id", raw: testAuthorityCommandRecordRawFor(1, nil, "verdict", testAuthorityRemoveField(testAuthorityCommandPayloadByKind("verdict"), `,"gateId":"gate_review"`), "pending")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := testAuthorityCommandCodecRoundTrip(test.raw); err == nil {
				t.Fatalf("accepted contradictory command record: %s", test.raw)
			}
		})
	}
}

func testAuthorityRecordCodecs() []testAuthorityCodec {
	return []testAuthorityCodec{
		{
			name:      "workspace-registry-jcs-v1",
			recordRaw: testAuthorityRegistryRecordRaw,
			roundTrip: func(raw []byte) (testAuthorityCodecResult, error) {
				record, err := decodeWorkspaceRegistryJCSV1(raw)
				if err != nil {
					return testAuthorityCodecResult{}, err
				}
				encoded, err := encodeWorkspaceRegistryJCSV1(record)
				return testAuthorityCodecResult{recordRev: record.RecordRev, priorGeneration: record.PriorGeneration, encoded: encoded}, err
			},
		},
		{
			name:      "workspace-authority-jcs-v1",
			recordRaw: testAuthorityWorkspaceRecordRaw,
			roundTrip: func(raw []byte) (testAuthorityCodecResult, error) {
				record, err := decodeWorkspaceAuthorityJCSV1(raw)
				if err != nil {
					return testAuthorityCodecResult{}, err
				}
				encoded, err := encodeWorkspaceAuthorityJCSV1(record)
				return testAuthorityCodecResult{recordRev: record.RecordRev, priorGeneration: record.PriorGeneration, encoded: encoded}, err
			},
		},
		{
			name:      "workspace-owner-lease-jcs-v1",
			recordRaw: testAuthorityOwnerLeaseRecordRaw,
			roundTrip: func(raw []byte) (testAuthorityCodecResult, error) {
				record, err := decodeWorkspaceOwnerLeaseJCSV1(raw)
				if err != nil {
					return testAuthorityCodecResult{}, err
				}
				encoded, err := encodeWorkspaceOwnerLeaseJCSV1(record)
				return testAuthorityCodecResult{recordRev: record.RecordRev, priorGeneration: record.PriorGeneration, encoded: encoded}, err
			},
		},
		{
			name:      "run-command-record-jcs-v1",
			recordRaw: testAuthorityCommandRecordRaw,
			roundTrip: testAuthorityCommandCodecRoundTrip,
		},
	}
}

func testAuthorityCommandCodecRoundTrip(raw []byte) (testAuthorityCodecResult, error) {
	record, err := decodeRunCommandRecordJCSV1(raw)
	if err != nil {
		return testAuthorityCodecResult{}, err
	}
	encoded, err := encodeRunCommandRecordJCSV1(record)
	return testAuthorityCodecResult{recordRev: record.RecordRev, priorGeneration: record.PriorGeneration, encoded: encoded}, err
}

func testAuthorityRegistryRecordRaw(recordRev uint64, prior *authorityGeneration) []byte {
	return testAuthorityRegistryRecordRawWithEntries(
		recordRev,
		prior,
		testAuthorityRegistryEntryRaw("/srv/work", "1", "2", testAuthorityRecordWorkspaceID, strings.Repeat("a", 64)),
	)
}

func testAuthorityRegistryRecordRawWithEntries(recordRev uint64, prior *authorityGeneration, entries ...[]byte) []byte {
	return []byte(fmt.Sprintf(
		`{"entries":[%s],"priorGeneration":%s,"recordRev":%d,"registrySchema":1}`,
		bytes.Join(entries, []byte(",")),
		testAuthorityPriorGenerationJSON(prior),
		recordRev,
	))
}

func testAuthorityRegistryEntryRaw(configuredPath, device, inode, workspaceAuthorityID, rootHash string) []byte {
	return []byte(fmt.Sprintf(
		`{"configuredPath":"%s","device":"%s","inode":"%s","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}`,
		configuredPath,
		device,
		inode,
		workspaceAuthorityID,
		rootHash,
	))
}

func testAuthorityWorkspaceRecordRaw(recordRev uint64, prior *authorityGeneration) []byte {
	return []byte(fmt.Sprintf(
		`{"admissionPolicyRef":%s,"authoritySchema":2,"nextAdmissionSeq":1,"nextWriterFence":1,"priorGeneration":%s,"recordRev":%d,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}`,
		testAuthorityAdmissionPolicyRefJSON(),
		testAuthorityPriorGenerationJSON(prior),
		recordRev,
		testAuthorityRecordWorkspaceID,
		strings.Repeat("a", 64),
	))
}

func testAuthorityAdmissionPolicyRefJSON() string {
	return fmt.Sprintf(`{"policyRev":1,"policySha256":"%s"}`, strings.Repeat("b", 64))
}

func testAuthorityOwnerLeaseRecordRaw(recordRev uint64, prior *authorityGeneration) []byte {
	return []byte(fmt.Sprintf(
		`{"acquiredAt":"2026-07-19T00:00:00Z","leaseSchema":1,"leaseUntil":"2026-07-19T00:00:30Z","ownerInstanceId":"owner-1","priorGeneration":%s,"recordRev":%d,"renewedAt":"2026-07-19T00:00:01Z","workspaceAuthorityId":"%s","writerFence":1}`,
		testAuthorityPriorGenerationJSON(prior),
		recordRev,
		testAuthorityRecordWorkspaceID,
	))
}

func testAuthorityCommandRecordRaw(recordRev uint64, prior *authorityGeneration) []byte {
	return testAuthorityCommandRecordRawFor(recordRev, prior, "cancel", testAuthorityCommandPayloadByKind("cancel"), "pending")
}

type testAuthorityCommandPayload struct {
	kind string
	raw  []byte
}

func testAuthorityCommandPayloads() []testAuthorityCommandPayload {
	return []testAuthorityCommandPayload{
		{
			kind: "start",
			raw: []byte(fmt.Sprintf(
				`{"actor":"agent:test","authoritySchema":2,"boardId":"brd_01J9_sesssearch","expectedBoardETag":"","expectedBoardRev":1,"kind":"start","limits":{"maxAttempts":1,"maxDispatch":1,"redact":true,"wallClockSeconds":60},"runRoot":{"kind":"mission","nodeId":"mis_01J9_improve"},"workspaceAuthorityId":"%s"}`,
				testAuthorityRecordWorkspaceID,
			)),
		},
		{
			kind: "resume",
			raw: []byte(fmt.Sprintf(
				`{"actor":"agent:test","authoritySchema":2,"blockedSeq":1,"kind":"resume","reason":"retry","resumeMode":"retry-failed-producer","runId":"%s","workspaceAuthorityId":"%s"}`,
				testAuthorityRecordRunID,
				testAuthorityRecordWorkspaceID,
			)),
		},
		{
			kind: "cancel",
			raw: []byte(fmt.Sprintf(
				`{"actor":"agent:test","authoritySchema":2,"expectedLastSeq":1,"kind":"cancel","reason":"","runId":"%s","workspaceAuthorityId":"%s"}`,
				testAuthorityRecordRunID,
				testAuthorityRecordWorkspaceID,
			)),
		},
		{
			kind: "verdict",
			raw: []byte(fmt.Sprintf(
				`{"actor":"agent:test","authoritySchema":2,"gateId":"gate_review","kind":"verdict","reason":"approved","requestedSeq":1,"runId":"%s","verdict":"pass","workspaceAuthorityId":"%s"}`,
				testAuthorityRecordRunID,
				testAuthorityRecordWorkspaceID,
			)),
		},
	}
}

func testAuthorityCommandPayloadByKind(kind string) []byte {
	for _, payload := range testAuthorityCommandPayloads() {
		if payload.kind == kind {
			return payload.raw
		}
	}
	panic("unknown test command payload kind: " + kind)
}

func testAuthorityCommandRecordRawFor(recordRev uint64, prior *authorityGeneration, commandKind string, payload []byte, state string) []byte {
	prefix := fmt.Sprintf(
		`{"admittedWriterFence":1,"commandEncoding":"run-command-jcs-v1","commandId":"%s","commandKind":"%s","commandPayload":%s,"commandPayloadSha256":"%s","commandSchema":1`,
		testAuthorityRecordCommandID,
		commandKind,
		payload,
		runtimeSHA256Hex(payload),
	)
	suffix := fmt.Sprintf(
		`,"priorGeneration":%s,"recordRev":%d`,
		testAuthorityPriorGenerationJSON(prior),
		recordRev,
	)
	switch state {
	case "pending":
		return []byte(fmt.Sprintf(`%s%s,"state":"pending","stateWriterFence":%d}`, prefix, suffix, recordRev))
	case "applied":
		return []byte(fmt.Sprintf(
			`%s,"decisionAdmissionPolicyRef":%s,"effectSeq":1,"outcomeWriterFence":1%s,"runId":"%s","state":"applied","stateWriterFence":%d}`,
			prefix,
			testAuthorityCommandDecisionRefJSON(commandKind),
			suffix,
			testAuthorityRecordRunID,
			recordRev,
		))
	case "rejected":
		return []byte(fmt.Sprintf(
			`%s,"decisionAdmissionPolicyRef":%s,"outcomeWriterFence":1%s,"rejectionCode":"rejected","state":"rejected","stateWriterFence":%d}`,
			prefix,
			testAuthorityCommandDecisionRefJSON(commandKind),
			suffix,
			recordRev,
		))
	default:
		return []byte(fmt.Sprintf(`%s%s,"state":"%s","stateWriterFence":%d}`, prefix, suffix, state, recordRev))
	}
}

func testAuthorityCommandDecisionRefJSON(commandKind string) string {
	if commandKind != "start" {
		return "null"
	}
	return testAuthorityAdmissionPolicyRefJSON()
}

func testAuthorityPriorGenerationJSON(prior *authorityGeneration) string {
	if prior == nil {
		return "null"
	}
	return fmt.Sprintf(`{"recordRev":%d,"sha256":"%s"}`, prior.recordRev, prior.sha256)
}

func testAuthorityRemovePriorGeneration(raw []byte, value string) []byte {
	return testAuthorityRemoveField(raw, `,"priorGeneration":`+value)
}

func testAuthorityCodecByName(name string) testAuthorityCodec {
	for _, codec := range testAuthorityRecordCodecs() {
		if codec.name == name {
			return codec
		}
	}
	panic("unknown test authority codec: " + name)
}

func testAuthorityReplaceOnce(raw, old, replacement []byte) []byte {
	if bytes.Count(raw, old) != 1 {
		panic(fmt.Sprintf("test fixture replacement count for %q in %q is not one", old, raw))
	}
	return bytes.Replace(raw, old, replacement, 1)
}

func testAuthorityRemoveField(raw []byte, field string) []byte {
	return testAuthorityReplaceOnce(raw, []byte(field), nil)
}

func testAuthorityInsertBefore(raw []byte, marker, field string) []byte {
	return testAuthorityReplaceOnce(raw, []byte(marker), []byte(field+marker))
}

func testAuthorityAppendObjectField(raw []byte, field string) []byte {
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		panic(fmt.Sprintf("test fixture is not an object: %q", raw))
	}
	result := make([]byte, 0, len(raw)+len(field)+1)
	result = append(result, raw[:len(raw)-1]...)
	result = append(result, ',')
	result = append(result, field...)
	return append(result, '}')
}

func testAuthorityInsertAfterObjectStart(raw []byte, field string) []byte {
	result := make([]byte, 0, len(raw)+len(field))
	result = append(result, '{')
	result = append(result, field...)
	return append(result, raw[1:]...)
}

func testAuthorityMoveFieldFirst(raw []byte, field string) []byte {
	withoutField := testAuthorityRemoveField(raw, field)
	return testAuthorityInsertAfterObjectStart(withoutField, strings.TrimPrefix(field, ",")+",")
}

func testAuthorityMoveRecordRevFirst(raw []byte) []byte {
	return testAuthorityMoveFieldFirst(raw, `,"recordRev":1`)
}
