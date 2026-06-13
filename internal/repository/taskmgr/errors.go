package taskmgr

import (
	"errors"

	"github.com/hk9890/task-manager/sdk/tasks"

	"github.com/hk9890/task-manager-ui/internal/domain"
	"github.com/hk9890/task-manager-ui/internal/repository"
)

func repoError(code domain.ErrorCode, op, message string, cause error) domain.RepositoryError {
	return domain.RepositoryError{Code: code, Operation: op, Message: message, Cause: cause}
}

// mapIssueErr normalizes errors from the single-issue Issue() read. An unknown
// ID becomes repository.ErrIssueNotFound — the local-state carve-out documented
// on the Repository interface and used by the memory backend. Other errors fall
// through to mapReadErr.
func mapIssueErr(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, tasks.ErrNotFound) {
		return repository.ErrIssueNotFound
	}
	return mapReadErr(op, err)
}

// mapReadErr normalizes errors from non-ID reads (Dashboard, Search, Catalogs,
// HealthCheck). "Not found" is not a meaningful outcome for these, so it is not
// special-cased here.
func mapReadErr(op string, err error) error {
	if err == nil {
		return nil
	}
	var ve *tasks.ValidationError
	var pe *tasks.ParseError
	switch {
	case errors.Is(err, tasks.ErrNoStore):
		return repoError(domain.ErrorCodeNoDatabaseFound, op, "", err)
	case errors.As(err, &ve):
		return repoError(domain.ErrorCodeValidationFailed, op, ve.Message, err)
	case errors.As(err, &pe):
		return repoError(domain.ErrorCodeValidationFailed, op, pe.Message, err)
	default:
		return repoError(domain.ErrorCodeUnknown, op, "", err)
	}
}

// mapWriteErr normalizes errors from write methods. To match the documented
// Repository interface behavior (and the parity contract), an unknown ID becomes
// a domain.RepositoryError (returned by value, so errors.As matches) with
// domain.ErrorCodeCommandFailed rather than the ErrIssueNotFound sentinel. An
// in-place edit of a closed issue (tasks.ErrImmutable) becomes
// domain.ErrorCodeConflict — a taskmgr-only outcome the memory backend lacks.
func mapWriteErr(op string, err error) error {
	if err == nil {
		return nil
	}
	var ve *tasks.ValidationError
	switch {
	case errors.Is(err, tasks.ErrNotFound):
		return repoError(domain.ErrorCodeCommandFailed, op, "issue not found", err)
	case errors.As(err, &ve):
		return repoError(domain.ErrorCodeValidationFailed, op, ve.Message, err)
	case errors.Is(err, tasks.ErrImmutable):
		return repoError(domain.ErrorCodeConflict, op, "issue is closed; reopen it before editing", err)
	case errors.Is(err, tasks.ErrNoStore):
		return repoError(domain.ErrorCodeNoDatabaseFound, op, "", err)
	default:
		return repoError(domain.ErrorCodeUnknown, op, "", err)
	}
}
