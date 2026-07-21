package formations_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/chrote/server/internal/formations"
)

func TestDefinitionPublicationUncertainSupportsExternalErrorsIs(t *testing.T) {
	callerErr := fmt.Errorf("caller context: %w", formations.ErrDefinitionPublicationUncertain)
	if !errors.Is(callerErr, formations.ErrDefinitionPublicationUncertain) {
		t.Fatalf("errors.Is(%v, ErrDefinitionPublicationUncertain) = false", callerErr)
	}
	if got := formations.ErrDefinitionPublicationUncertain.Error(); got != "definition_publication_uncertain" {
		t.Fatalf("ErrDefinitionPublicationUncertain.Error() = %q, want stable code", got)
	}
}
