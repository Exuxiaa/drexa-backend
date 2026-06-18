package apperr

import "errors"

// Sentinel errors used across domain boundaries.
// Each domain defines its own specific errors; these are the generic transport layer.
var (
	ErrNotFound   = errors.New("not found")
	ErrForbidden  = errors.New("forbidden")
	ErrConflict   = errors.New("conflict")
	ErrBadRequest = errors.New("bad request")
	ErrInternal   = errors.New("internal error")
	ErrUnauthorized = errors.New("unauthorized")
)
