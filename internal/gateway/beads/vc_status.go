package beads

import (
	"context"
	"strings"

	"github.com/hk9890/beads-workbench/internal/domain"
)

// bdVCStatusPayload is the typed target for `bd vc status --json` output.
// Only the commit field is required; other fields in the JSON response are
// intentionally omitted to follow CODING.md rule #10 (typed, operation-scoped decoding).
type bdVCStatusPayload struct {
	Commit string `json:"commit"`
}

// VCStatusHash returns the current Dolt commit hash from `bd vc status`.
// Returns ("", err) on any failure; the caller treats error as "skip this tick".
// Two consecutive calls with no external bd writes return the same hash.
// Calls that succeed but cannot parse a hash return an error.
func VCStatusHash(ctx context.Context, runner *CommandRunner) (string, error) {
	const operation = "vc status"

	payload, err := RunJSON[bdVCStatusPayload](ctx, runner, CommandRequest{
		Operation: operation,
		Args:      []string{"vc", "status", "--json"},
		IsWrite:   false,
	})
	if err != nil {
		return "", err
	}

	hash := strings.TrimSpace(payload.Commit)
	if hash == "" {
		return "", newGatewayError(domain.ErrorCodeDecodeFailed, operation, "bd vc status: commit hash is empty", nil)
	}

	return hash, nil
}
