package formations

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

const (
	testAuthorityRecordWorkspaceID = "wsa_01KXNP6VY3227H78329V52CKF8"
	testAuthorityRecordCommandID   = "cmd_01KXNP6VY3227H78329V52CKF8"
	testAuthorityRecordRunID       = "run_01KXNP6VY3227H78329V52CKF8"
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
				{name: "duplicate key", raw: testAuthorityInsertAfterObjectStart(firstRaw, `"recordRev":1,`)},
				{name: "unknown key", raw: testAuthorityInsertAfterObjectStart(firstRaw, `"unknown":true,`)},
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

func TestRunCommandRecordCodecRejectsUnknownStateWithoutExecutingCommand(t *testing.T) {
	raw := testAuthorityCommandRecordRaw(1, nil)
	wrongState := bytes.Replace(raw, []byte(`"state":"pending"`), []byte(`"state":"completed"`), 1)
	if bytes.Equal(wrongState, raw) {
		t.Fatal("wrong-state fixture did not change command bytes")
	}
	if _, err := testAuthorityCommandCodecRoundTrip(wrongState); err == nil {
		t.Fatalf("accepted unknown command-record state: %s", wrongState)
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
	return []byte(fmt.Sprintf(
		`{"entries":[{"configuredPath":"/srv/work","device":"1","inode":"2","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}],"priorGeneration":%s,"recordRev":%d,"registrySchema":1}`,
		testAuthorityRecordWorkspaceID,
		strings.Repeat("a", 64),
		testAuthorityPriorGenerationJSON(prior),
		recordRev,
	))
}

func testAuthorityWorkspaceRecordRaw(recordRev uint64, prior *authorityGeneration) []byte {
	return []byte(fmt.Sprintf(
		`{"admissionPolicyRef":{"policyRev":1,"policySha256":"%s"},"authoritySchema":2,"nextAdmissionSeq":1,"nextWriterFence":1,"priorGeneration":%s,"recordRev":%d,"rootIdentityEncoding":"workspace-root-identity-v1","workspaceAuthorityId":"%s","workspaceRootIdentitySha256":"%s"}`,
		strings.Repeat("b", 64),
		testAuthorityPriorGenerationJSON(prior),
		recordRev,
		testAuthorityRecordWorkspaceID,
		strings.Repeat("a", 64),
	))
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
	payload := []byte(fmt.Sprintf(
		`{"actor":"agent:test","authoritySchema":2,"expectedLastSeq":1,"kind":"cancel","reason":"","runId":"%s","workspaceAuthorityId":"%s"}`,
		testAuthorityRecordRunID,
		testAuthorityRecordWorkspaceID,
	))
	return []byte(fmt.Sprintf(
		`{"admittedWriterFence":1,"commandEncoding":"run-command-jcs-v1","commandId":"%s","commandKind":"cancel","commandPayload":%s,"commandPayloadSha256":"%s","commandSchema":1,"priorGeneration":%s,"recordRev":%d,"state":"pending","stateWriterFence":%d}`,
		testAuthorityRecordCommandID,
		payload,
		runtimeSHA256Hex(payload),
		testAuthorityPriorGenerationJSON(prior),
		recordRev,
		recordRev,
	))
}

func testAuthorityPriorGenerationJSON(prior *authorityGeneration) string {
	if prior == nil {
		return "null"
	}
	return fmt.Sprintf(`{"recordRev":%d,"sha256":"%s"}`, prior.recordRev, prior.sha256)
}

func testAuthorityRemovePriorGeneration(raw []byte, value string) []byte {
	return bytes.Replace(raw, []byte(`,"priorGeneration":`+value), nil, 1)
}

func testAuthorityInsertAfterObjectStart(raw []byte, field string) []byte {
	result := make([]byte, 0, len(raw)+len(field))
	result = append(result, '{')
	result = append(result, field...)
	return append(result, raw[1:]...)
}

func testAuthorityMoveRecordRevFirst(raw []byte) []byte {
	withoutRecordRev := bytes.Replace(raw, []byte(`,"recordRev":1`), nil, 1)
	if bytes.Equal(withoutRecordRev, raw) {
		return raw
	}
	return testAuthorityInsertAfterObjectStart(withoutRecordRev, `"recordRev":1,`)
}
