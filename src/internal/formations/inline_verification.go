package formations

import (
	"errors"
	"fmt"
)

const LegacyInlineVerificationMigrationCode = "legacy_inline_verification_requires_migration"

var ErrLegacyInlineVerificationRequiresMigration = errors.New(LegacyInlineVerificationMigrationCode)

func rejectLegacyInlineVerification(board *BoardDocument) error {
	if board == nil {
		return nil
	}
	for _, formation := range board.Formations {
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
