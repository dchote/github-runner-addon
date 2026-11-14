package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/dchote/github-runner-addon/internal/container/docker"
	"github.com/dchote/github-runner-addon/internal/store"
)

// jobStartedHookScript writes busy status.json. Always exits 0.
const jobStartedHookScript = `#!/bin/bash
# Managed by github-runner-addon — do not edit. Always exit 0.
status_dir="$(cd "$(dirname "$0")/.." && pwd)"
status_file="${status_dir}/status.json"
tmp="${status_file}.tmp.$$"
esc() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }
now="$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || true)"
{
  printf '{'
  printf '"busy":true'
  printf ',"repository":"%s"' "$(esc "${GITHUB_REPOSITORY:-}")"
  printf ',"workflow":"%s"' "$(esc "${GITHUB_WORKFLOW:-}")"
  printf ',"job":"%s"' "$(esc "${GITHUB_JOB:-}")"
  printf ',"run_id":"%s"' "$(esc "${GITHUB_RUN_ID:-}")"
  printf ',"run_number":"%s"' "$(esc "${GITHUB_RUN_NUMBER:-}")"
  printf ',"run_attempt":"%s"' "$(esc "${GITHUB_RUN_ATTEMPT:-}")"
  printf ',"sha":"%s"' "$(esc "${GITHUB_SHA:-}")"
  printf ',"ref":"%s"' "$(esc "${GITHUB_REF:-}")"
  printf ',"actor":"%s"' "$(esc "${GITHUB_ACTOR:-}")"
  printf ',"event":"%s"' "$(esc "${GITHUB_EVENT_NAME:-}")"
  printf ',"updated_at":"%s"' "$(esc "${now}")"
  printf '}\n'
} >"$tmp" 2>/dev/null && mv -f "$tmp" "$status_file" 2>/dev/null || rm -f "$tmp" 2>/dev/null
exit 0
`

// jobCompletedHookScript clears busy. Always exits 0.
const jobCompletedHookScript = `#!/bin/bash
# Managed by github-runner-addon — do not edit. Always exit 0.
status_dir="$(cd "$(dirname "$0")/.." && pwd)"
status_file="${status_dir}/status.json"
tmp="${status_file}.tmp.$$"
now="$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || true)"
printf '{"busy":false,"updated_at":"%s"}\n' "$now" >"$tmp" 2>/dev/null && mv -f "$tmp" "$status_file" 2>/dev/null || rm -f "$tmp" 2>/dev/null
exit 0
`

func (m *Manager) installJobHooks(ctx context.Context, workdir string) error {
	if err := m.ensureJobHookScripts(ctx, workdir); err != nil {
		return err
	}
	return m.seedIdleJobStatus(ctx, workdir)
}

func (m *Manager) ensureJobHookScripts(ctx context.Context, workdir string) error {
	wh := m.workdirHostOrDocker()
	if wh == nil {
		return ErrDockerUnavailable
	}
	if workdir == "" || workdir == "/" {
		return fmt.Errorf("invalid workdir for job hooks")
	}
	addonDir := jobWorkdirPath(workdir, jobAddonDirRel)
	hooksDir := jobWorkdirPath(workdir, jobHooksDirRel)
	if err := wh.EnsureHostDir(ctx, hooksDir); err != nil {
		return fmt.Errorf("ensure job hooks dir: %w", err)
	}
	// World-writable so non-root runner uid can update status.json (any job can write it).
	if err := wh.ChmodHostPath(ctx, addonDir, 0o777); err != nil {
		slog.Warn("chmod job addon dir", "path", addonDir, "err", err)
	}
	if err := wh.WriteHostFile(ctx, jobStartedHookPath(workdir), []byte(jobStartedHookScript), 0o755); err != nil {
		return fmt.Errorf("write job-started hook: %w", err)
	}
	if err := wh.WriteHostFile(ctx, jobCompletedHookPath(workdir), []byte(jobCompletedHookScript), 0o755); err != nil {
		return fmt.Errorf("write job-completed hook: %w", err)
	}
	return nil
}

func (m *Manager) seedIdleJobStatus(ctx context.Context, workdir string) error {
	wh := m.workdirHostOrDocker()
	if wh == nil {
		return ErrDockerUnavailable
	}
	if workdir == "" || workdir == "/" {
		return fmt.Errorf("invalid workdir for job status")
	}
	if err := wh.WriteHostFile(ctx, jobStatusHostPath(workdir), idleJobStatusJSON(time.Now()), 0o666); err != nil {
		return fmt.Errorf("seed job status: %w", err)
	}
	m.invalidateJobStatusCache(workdir)
	return nil
}

func (m *Manager) applyJobStatus(ctx context.Context, v *View, containerRef, workdir string, live bool) {
	data, err := m.readJobStatusBytes(ctx, containerRef, workdir, live)
	if err != nil {
		if !isJobStatusMissing(err) && !docker.IsContextError(err) {
			slog.Debug("job status read failed", "container", containerRef, "err", err)
		}
		v.JobState = jobStateUnknown
		return
	}
	state, job := parseJobStatusFile(data, time.Now())
	v.JobState = state
	if state == jobStateBusy {
		v.CurrentJob = job
	}
}

// errIfBusy rejects destructive lifecycle ops while a job is in progress so
// recreate/delete/apply cannot tear down the agent mid-build (GitHub then
// reports "self-hosted runner lost communication").
func (m *Manager) errIfBusy(ctx context.Context, rec store.Runner) error {
	running, known, err := m.containerIsRunning(ctx, rec.ContainerName)
	if err != nil {
		return wrapInspectErr(err)
	}
	if known && !running {
		// Missing/exited: leftover host status.json must not block recreate after a Docker reset.
		return nil
	}
	workdir := resolveWorkdirHostPath(rec)
	if workdir == "" {
		return nil
	}
	m.invalidateJobStatusCache(workdir)
	data, err := m.readJobStatusBytes(ctx, rec.ContainerName, workdir, true)
	if err != nil {
		if known && running {
			return fmt.Errorf("%w: cannot verify idle; stop the runner first", ErrRunnerBusy)
		}
		// Tests without inspect (known=false) fall through to host status.json miss as idle.
		return nil
	}
	state, job := parseJobStatusFile(data, time.Now())
	if state == jobStateBusy {
		detail := "a job is running"
		if job != nil {
			parts := make([]string, 0, 3)
			if job.Repository != "" {
				parts = append(parts, job.Repository)
			}
			if job.Workflow != "" {
				parts = append(parts, job.Workflow)
			}
			if job.Job != "" {
				parts = append(parts, job.Job)
			}
			if len(parts) > 0 {
				detail = strings.Join(parts, " / ")
			}
		}
		return fmt.Errorf("%w: refuse recreate/delete/apply while busy (%s); wait for the job to finish or stop the runner first", ErrRunnerBusy, detail)
	}
	if known && running && state == jobStateUnknown {
		return fmt.Errorf("%w: cannot verify idle; stop the runner first", ErrRunnerBusy)
	}
	return nil
}

// containerIsRunning reports (running, known, err).
// known is false when there is no inspect source (tests without Docker); callers then fall through to status.json.
// Inspect errors fail closed (err != nil) so a flaky daemon cannot skip the busy gate then kill a running job.
func (m *Manager) containerIsRunning(ctx context.Context, name string) (running bool, known bool, err error) {
	if name == "" {
		return false, true, nil
	}
	info, err := m.inspectContainer(ctx, name)
	if err != nil {
		if errors.Is(err, errInspectUnavailable) {
			return false, false, nil
		}
		return false, false, err
	}
	return info.Exists && info.Running, true, nil
}

func (m *Manager) readJobStatusBytes(ctx context.Context, containerRef, workdir string, live bool) ([]byte, error) {
	statusPath := jobStatusHostPath(workdir)
	if cached, ok := m.getCachedJobStatus(statusPath); ok {
		if cached.miss {
			return nil, docker.ErrHostFileNotFound
		}
		if cached.err != nil {
			return nil, cached.err
		}
		return cached.raw, nil
	}

	data, err := m.fetchJobStatusBytes(ctx, containerRef, statusPath, live)
	m.putCachedJobStatus(statusPath, data, err)
	return data, err
}

// fetchJobStatusBytes reads status.json. List mode (live=false) uses cache +
// CopyFromContainer only — no alpine host-file helpers (avoids N helper storms).
// Get/lifecycle (live=true) may fall back to ReadHostFile.
func (m *Manager) fetchJobStatusBytes(ctx context.Context, containerRef, statusPath string, live bool) ([]byte, error) {
	if containerRef != "" && m.Docker != nil {
		data, err := m.Docker.ReadContainerFile(ctx, containerRef, statusPath)
		if err == nil {
			return data, nil
		}
		if !live {
			return nil, err
		}
		if !isJobStatusMissing(err) && !docker.IsContextError(err) {
			slog.Debug("CopyFromContainer job status failed; trying host read", "path", statusPath, "err", err)
		}
	} else if !live {
		return nil, docker.ErrHostFileNotFound
	}
	wh := m.workdirHostOrDocker()
	if wh == nil {
		return nil, ErrDockerUnavailable
	}
	return wh.ReadHostFile(ctx, statusPath)
}

func (m *Manager) getCachedJobStatus(key string) (jobStatusCacheEntry, bool) {
	m.jobStatusMu.RLock()
	defer m.jobStatusMu.RUnlock()
	if m.jobStatusCache == nil {
		return jobStatusCacheEntry{}, false
	}
	e, ok := m.jobStatusCache[key]
	if !ok || time.Since(e.at) > jobStatusCacheTTL {
		return jobStatusCacheEntry{}, false
	}
	return e, true
}

func (m *Manager) putCachedJobStatus(key string, data []byte, err error) {
	m.jobStatusMu.Lock()
	defer m.jobStatusMu.Unlock()
	if m.jobStatusCache == nil {
		m.jobStatusCache = make(map[string]jobStatusCacheEntry)
	}
	e := jobStatusCacheEntry{at: time.Now()}
	if err != nil {
		e.err = err
		e.miss = isJobStatusMissing(err)
	} else {
		e.raw = append([]byte(nil), data...)
	}
	m.jobStatusCache[key] = e
}

func (m *Manager) invalidateJobStatusCache(workdir string) {
	if m == nil {
		return
	}
	key := jobStatusHostPath(workdir)
	m.jobStatusMu.Lock()
	if m.jobStatusCache != nil {
		delete(m.jobStatusCache, key)
	}
	m.jobStatusMu.Unlock()
}

func isJobStatusMissing(err error) bool {
	return errors.Is(err, docker.ErrHostFileNotFound) || errors.Is(err, docker.ErrContainerFileNotFound)
}

// ensureHooksThenStart starts a stopped container after refreshing hook scripts.
// If already running, returns without mutating status.json (avoids wiping busy).
// Seeds idle only after a successful transition from stopped → started.
func (m *Manager) ensureHooksThenStart(ctx context.Context, rec store.Runner) error {
	workdir := resolveWorkdirHostPath(rec)
	info, err := m.inspectContainer(ctx, rec.ContainerName)
	if err != nil {
		return wrapInspectErr(err)
	}
	if info.Exists && info.Running {
		return nil
	}
	if !info.Exists {
		return fmt.Errorf("%w: container is missing; recreate it", ErrValidation)
	}
	if m.Docker == nil {
		return ErrDockerUnavailable
	}
	if err := m.ensureJobHookScripts(ctx, workdir); err != nil {
		slog.Debug("ensure job hooks before start", "runner", rec.ID, "err", err)
	}
	if err := m.Docker.Start(ctx, rec.ContainerName); err != nil {
		return err
	}
	if err := m.seedIdleJobStatus(ctx, workdir); err != nil {
		slog.Debug("seed idle job status after start", "runner", rec.ID, "err", err)
	}
	return nil
}

// ensureHooksThenRestart restarts the container; seeds idle only after success.
func (m *Manager) ensureHooksThenRestart(ctx context.Context, rec store.Runner) error {
	workdir := resolveWorkdirHostPath(rec)
	info, err := m.inspectContainer(ctx, rec.ContainerName)
	if err != nil {
		return wrapInspectErr(err)
	}
	if !info.Exists {
		return fmt.Errorf("%w: container is missing; recreate it", ErrValidation)
	}
	if m.Docker == nil {
		return ErrDockerUnavailable
	}
	if err := m.ensureJobHookScripts(ctx, workdir); err != nil {
		slog.Debug("ensure job hooks before restart", "runner", rec.ID, "err", err)
	}
	if err := m.Docker.Restart(ctx, rec.ContainerName); err != nil {
		return err
	}
	if err := m.seedIdleJobStatus(ctx, workdir); err != nil {
		slog.Debug("seed idle job status after restart", "runner", rec.ID, "err", err)
	}
	return nil
}
