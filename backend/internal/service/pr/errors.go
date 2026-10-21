package pr

import "errors"

// Sentinel errors returned by the PR action service.
var (
	ErrInvalidPR        = errors.New("pr: invalid identity")
	ErrPRNotFound       = errors.New("pr: not found")
	ErrPRNotMergeable   = errors.New("pr: not mergeable")
	ErrPRHeadChanged    = errors.New("pr: head changed")
	ErrPRPreconditions  = errors.New("pr: merge preconditions unmet")
	ErrNothingToResolve = errors.New("pr: nothing to resolve")
	// ErrPRVerifyFailed means the project's verify command did not pass on the
	// exact head being merged, so the merge was refused.
	ErrPRVerifyFailed = errors.New("pr: verification failed")
)
