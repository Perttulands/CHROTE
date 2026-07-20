package formations

import (
	"errors"
	"fmt"
)

const LegacyInlineVerificationMigrationCode = "legacy_inline_verification_requires_migration"

var ErrLegacyInlineVerificationRequiresMigration = errors.New(LegacyInlineVerificationMigrationCode)

func rejectLegacyInlineVerification(board *BoardDocument) error {
	return rejectLegacyInlineVerificationForNodes(board, nil)
}

func rejectLegacyInlineVerificationForNodes(board *BoardDocument, selected map[string]bool) error {
	if board == nil {
		return nil
	}
	for _, formation := range board.Formations {
		if selected != nil && !selected[formation.ID] {
			continue
		}
		if formation.Verification == nil {
			continue
		}
		return fmt.Errorf(
			"%w: formation %q uses retired inline verification; create and wire an explicit Gate, then remove the legacy verification",
			ErrLegacyInlineVerificationRequiresMigration,
			formation.ID,
		)
	}
	return nil
}
