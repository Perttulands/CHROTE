package api

import "testing"

// ADR-0014 decision 3 adds exactly one cell to ADR-0001's mode/owner matrix:
// an agent-mode workload owned by an external manager may restart, but only when
// that manager is a unit CHROTE installed. The tests here pin both halves of
// "CHROTE installed": the name must match the template we own (checkable here,
// on a pure descriptor), and our config must exist (checkable only against the
// filesystem, pinned in TestAgentUnitController_OnlyChroteInstalledUnitsMayClaimRestartCapability).
//
// The failure this prevents: a hand-written unit that borrows our naming scheme,
// or any other external manager, being handed restart authority by claiming it.

func TestRecoveryMatrix_ChroteInstalledUnitMayRestartAnAgentWorkload(t *testing.T) {
	owner := WorkloadRecoveryOwner{
		Kind:       RecoveryOwnerExternalManager,
		Ref:        "systemd:user/chrote-agent@codex-alpha.service",
		MayRestart: true,
	}
	if err := validateRecoveryModeOwner(RecoveryModeAgent, owner); err != nil {
		t.Fatalf("a CHROTE-installed unit must be allowed to restart its agent: %v", err)
	}
}

func TestRecoveryMatrix_ForeignExternalManagerStillCannotClaimRestart(t *testing.T) {
	foreign := []string{
		"systemd:user/codex-minerva-telegram.service",
		"systemd:user/chrote-agent-codex-alpha.service",
		"systemd:user/chrote-agent@codex-alpha.timer",
		"systemd:system/chrote-agent@codex-alpha.service",
		"systemd:user/evil@chrote-agent@codex-alpha.service",
		"chrote-agent@codex-alpha.service",
		"",
	}
	for _, ref := range foreign {
		owner := WorkloadRecoveryOwner{
			Kind:       RecoveryOwnerExternalManager,
			Ref:        ref,
			MayRestart: true,
		}
		if err := validateRecoveryModeOwner(RecoveryModeAgent, owner); err == nil {
			t.Fatalf("ref %q is not a CHROTE-installed unit and must not claim restart", ref)
		}
	}
}

func TestRecoveryMatrix_ManagedModeIsUnchangedAndStaysReadOnly(t *testing.T) {
	// The read-only contract for sessions someone else owns is exactly as
	// ADR-0001 wrote it; ADR-0014 narrows nothing here.
	owner := WorkloadRecoveryOwner{
		Kind:       RecoveryOwnerExternalManager,
		Ref:        "systemd:user/chrote-agent@codex-alpha.service",
		MayRestart: true,
	}
	if err := validateRecoveryModeOwner(RecoveryModeManaged, owner); err == nil {
		t.Fatal("managed descriptors must still require a non-restarting owner, even for our own units")
	}
}

func TestRecoveryMatrix_SessionBankAgentOwnershipIsUnchanged(t *testing.T) {
	owner := WorkloadRecoveryOwner{
		Kind:       RecoveryOwnerSessionBank,
		Ref:        "session_bank:alice/codex-alpha",
		MayRestart: true,
	}
	if err := validateRecoveryModeOwner(RecoveryModeAgent, owner); err != nil {
		t.Fatalf("session bank agent ownership must keep working: %v", err)
	}
	owner.MayRestart = false
	if err := validateRecoveryModeOwner(RecoveryModeAgent, owner); err == nil {
		t.Fatal("an agent owner that cannot restart is still invalid")
	}
}
