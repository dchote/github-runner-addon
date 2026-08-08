package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

func (m *Manager) applyJobStatus(ctx context.Context, v *View, containerRef, workdir string) {
	data, err := m.readJobStatusBytes(ctx, containerRef, workdir)
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

func (m *Manager) readJobStatusBytes(ctx context.Context, containerRef, workdir string) ([]byte, error) {
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

	data, err := m.fetchJobStatusBytes(ctx, containerRef, statusPath)
	m.putCachedJobStatus(statusPath, data, err)
	return data, err
}

func (m *Manager) fetchJobStatusBytes(ctx context.Context, containerRef, statusPath string) ([]byte, error) {
	if containerRef != "" && m.Docker != nil {
		data, err := m.Docker.ReadContainerFile(ctx, containerRef, statusPath)
		if err == nil {
			return data, nil
		}
		if !isJobStatusMissing(err) && !docker.IsContextError(err) {
			slog.Debug("CopyFromContainer job status failed; trying host read", "path", statusPath, "err", err)
		}
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
	if m.Docker == nil {
		return ErrDockerUnavailable
	}
	info, err := m.Docker.InspectByName(ctx, rec.ContainerName)
	if err != nil {
		return err
	}
	if info.Exists && info.Running {
		return nil
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
