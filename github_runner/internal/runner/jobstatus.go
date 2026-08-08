package runner

import (
	"bytes"
	"encoding/json"
	"strings"
	"time"
)

const (
	jobAddonDirRel      = ".gha-addon"
	jobHooksDirRel      = jobAddonDirRel + "/hooks"
	jobStatusFileRel    = jobAddonDirRel + "/status.json"
	jobStartedHookRel   = jobHooksDirRel + "/job-started.sh"
	jobCompletedHookRel = jobHooksDirRel + "/job-completed.sh"

	jobStateIdle    = "idle"
	jobStateBusy    = "busy"
	jobStateUnknown = "unknown"

	// Demote stuck busy after this age (killed worker / missed completed hook).
	jobBusyStaleAfter = 24 * time.Hour

	// Short TTL for list/get polling so CopyFromContainer is not per-refresh spam.
	jobStatusCacheTTL = 2 * time.Second
)

// CurrentJob is job metadata written by Actions job hooks (no GitHub API).
type CurrentJob struct {
	Repository string `json:"repository,omitempty"`
	Workflow   string `json:"workflow,omitempty"`
	Job        string `json:"job,omitempty"`
	RunID      string `json:"run_id,omitempty"`
	RunNumber  string `json:"run_number,omitempty"`
	RunAttempt string `json:"run_attempt,omitempty"`
	SHA        string `json:"sha,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Actor      string `json:"actor,omitempty"`
	Event      string `json:"event,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type jobStatusFile struct {
	Busy bool `json:"busy"`
	CurrentJob
}

type jobStatusCacheEntry struct {
	at   time.Time
	raw  []byte
	err  error
	miss bool
}

func jobWorkdirPath(workdir, rel string) string {
	return strings.TrimRight(workdir, "/") + "/" + rel
}

func jobStatusHostPath(workdir string) string {
	return jobWorkdirPath(workdir, jobStatusFileRel)
}

func jobStartedHookPath(workdir string) string {
	return jobWorkdirPath(workdir, jobStartedHookRel)
}

func jobCompletedHookPath(workdir string) string {
	return jobWorkdirPath(workdir, jobCompletedHookRel)
}

func idleJobStatusJSON(now time.Time) []byte {
	b, _ := json.Marshal(jobStatusFile{
		Busy: false,
		CurrentJob: CurrentJob{
			UpdatedAt: now.UTC().Format(time.RFC3339),
		},
	})
	return append(b, '\n')
}

// parseJobStatusFile maps status.json bytes to job_state + optional current_job.
// Empty or invalid JSON → unknown. Busy without a fresh updated_at → unknown.
func parseJobStatusFile(data []byte, now time.Time) (jobState string, job *CurrentJob) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return jobStateUnknown, nil
	}
	var st jobStatusFile
	if err := json.Unmarshal(data, &st); err != nil {
		return jobStateUnknown, nil
	}
	if !st.Busy {
		return jobStateIdle, nil
	}
	updated, err := time.Parse(time.RFC3339, st.UpdatedAt)
	if err != nil {
		// Fail closed: busy without a trustworthy timestamp is not actionable.
		return jobStateUnknown, nil
	}
	if now.Sub(updated) > jobBusyStaleAfter {
		return jobStateUnknown, nil
	}
	j := st.CurrentJob
	return jobStateBusy, &j
}
