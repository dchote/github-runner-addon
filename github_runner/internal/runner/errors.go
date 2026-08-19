package runner

import "errors"

var (
	ErrValidation        = errors.New("validation error")
	ErrDockerUnavailable = errors.New("docker unavailable")
	ErrGitHub            = errors.New("github api error")
	ErrRateLimited       = errors.New("rate limited")
	// ErrImagePull is a registry/network failure while pulling a runner image (not a missing :local tag).
	ErrImagePull = errors.New("image pull failed")
	// ErrRunnerBusy is returned when recreate/delete/apply would kill an in-flight job.
	ErrRunnerBusy = errors.New("runner busy")
)
