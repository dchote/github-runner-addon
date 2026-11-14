package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dchote/github-runner-addon/internal/container/docker"
	"github.com/dchote/github-runner-addon/internal/store"
)

const (
	// DefaultImageRefreshInterval is how often the fleet is checked and rebuilt
	// onto the configured runner image. Upstream tags such as :latest move more
	// often than the Actions agent itself; a week keeps runners current without
	// recreating them on every base-image push. Addon option
	// runner_image_refresh_days overrides this. Zero disables the check.
	DefaultImageRefreshInterval = 7 * 24 * time.Hour
	// runnerImageRefreshRetry is the backoff after a registry pull fails.
	runnerImageRefreshRetry = 15 * time.Minute
)

// rebuildPlan is the per-runner decision for a fleet rebuild pass.
type rebuildPlan struct {
	Recreate    bool
	Reconfigure bool
	Retarget    bool // stored image name changed; container is already on that image
	Warn        bool
}

// planRunnerRebuild decides whether a runner container is replaced.
// active is true when the container is running, or the operator is starting it.
// stale means its image ID differs from the pulled target.
// retarget means the stored ref is an older tag of the same image.
// updatesDisabled means .runner was configured with --disableupdate.
func planRunnerRebuild(active, stale, retarget, updatesDisabled, patConfigured bool) rebuildPlan {
	if !active {
		return rebuildPlan{}
	}
	reconfigure := updatesDisabled && patConfigured
	if stale || reconfigure {
		return rebuildPlan{
			Recreate:    true,
			Reconfigure: reconfigure,
			Warn:        updatesDisabled && !patConfigured,
		}
	}
	if retarget {
		return rebuildPlan{Retarget: true, Warn: updatesDisabled && !patConfigured}
	}
	if updatesDisabled {
		return rebuildPlan{Warn: true}
	}
	return rebuildPlan{}
}

// refreshImageRef is the image a runner should run. Runners on the configured
// image, or any other tag of that same repository, roll forward to it. :local
// and unrelated repositories stay on their own ref.
func refreshImageRef(configured, stored string) (ref string, fleet bool) {
	target := strings.TrimSpace(configured)
	if target == "" {
		target = DefaultRunnerImage
	}
	stored = strings.TrimSpace(stored)
	if docker.IsLocalOnlyImage(target) {
		if stored == "" {
			return target, false
		}
		return stored, false
	}
	if docker.IsLocalOnlyImage(stored) {
		return stored, false
	}
	if stored == "" || imageRepository(stored) == imageRepository(target) {
		return target, true
	}
	return stored, false
}

// imageRepository is the image name without tag or digest.
func imageRepository(ref string) string {
	ref = strings.TrimSpace(ref)
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	slash := strings.LastIndex(ref, "/")
	colon := strings.LastIndex(ref, ":")
	if colon > slash {
		ref = ref[:colon]
	}
	ref = strings.TrimPrefix(ref, "docker.io/")
	ref = strings.TrimPrefix(ref, "index.docker.io/")
	return ref
}

// runnerImageStale reports whether the container was created from a different
// image ID than the ref now resolves to. Non-IDs (image names) do not match,
// so a daemon that echoes the tag cannot force a recreate every pass.
func runnerImageStale(containerImageID, currentImageID string) bool {
	a := normalizeImageID(containerImageID)
	b := normalizeImageID(currentImageID)
	if !looksLikeImageID(a) || !looksLikeImageID(b) {
		return false
	}
	return a != b
}

func normalizeImageID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	id = strings.TrimPrefix(id, "sha256:")
	return id
}

func looksLikeImageID(id string) bool {
	if len(id) != 64 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		default:
			return false
		}
	}
	return true
}

// runnerAutoUpdateDisabled reports whether .runner was saved with disableUpdate.
// GitHub's runner JSON is camelCase; accept the PascalCase form as well.
func runnerAutoUpdateDisabled(raw []byte) bool {
	raw = bytes.TrimPrefix(bytes.TrimSpace(raw), []byte{0xEF, 0xBB, 0xBF})
	if len(raw) == 0 {
		return false
	}
	var cfg struct {
		DisableUpdate  bool `json:"disableUpdate"`
		DisableUpdate2 bool `json:"DisableUpdate"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return false
	}
	return cfg.DisableUpdate || cfg.DisableUpdate2
}

func (m *Manager) readRunnerConfig(ctx context.Context, volumeName string) ([]byte, error) {
	if strings.TrimSpace(volumeName) == "" {
		return nil, errNoRunnerConfig
	}
	wh := m.workdirHostOrDocker()
	if wh == nil {
		return nil, ErrDockerUnavailable
	}
	raw, err := wh.ReadVolumeFile(ctx, volumeName, runnerConfigFile)
	if err != nil {
		if errors.Is(err, docker.ErrVolumeFileNotFound) {
			return nil, errNoRunnerConfig
		}
		return nil, err
	}
	return raw, nil
}

func (m *Manager) warnAutoUpdateOnce(rec store.Runner) {
	if m.warnedAutoUpdate == nil {
		m.warnedAutoUpdate = map[string]struct{}{}
	}
	if _, ok := m.warnedAutoUpdate[rec.ID]; ok {
		return
	}
	m.warnedAutoUpdate[rec.ID] = struct{}{}
	slog.Warn("runner auto-update is disabled; GitHub will reject this Actions runner version again when it is deprecated. Set GITHUB_PAT, or recreate with a registration token, so the runner can be reconfigured with auto-update",
		"runner", rec.Name, "id", rec.ID)
}

// runnerJobBusy reports an in-flight job. Unreadable status is not busy:
// a rebuild must still replace a deprecated agent that is up but idle-unknown.
func (m *Manager) runnerJobBusy(ctx context.Context, rec store.Runner) (bool, error) {
	running, known, err := m.containerIsRunning(ctx, rec.ContainerName)
	if err != nil {
		return false, err
	}
	if known && !running {
		return false, nil
	}
	workdir := resolveWorkdirHostPath(rec)
	if workdir == "" {
		return false, nil
	}
	m.invalidateJobStatusCache(workdir)
	data, err := m.readJobStatusBytes(ctx, rec.ContainerName, workdir, true)
	if err != nil {
		return false, nil
	}
	state, _ := parseJobStatusFile(data, time.Now())
	return state == jobStateBusy, nil
}

// maybeRefreshRunnerImages rebuilds running runners onto the configured image
// when a newer build is available. The check runs at most once per configured
// interval (default one week) and that timestamp is kept across addon restarts.
// Busy runners are retried on the next reconcile tick without pulling again.
// A failed pull is retried after runnerImageRefreshRetry. Stopped runners are
// left stopped; Start and Restart do not pull a new image.
func (m *Manager) maybeRefreshRunnerImages(ctx context.Context) {
	if m == nil || m.Store == nil || m.imageRefreshEvery <= 0 {
		return
	}
	target, _ := refreshImageRef(m.Image, "")
	m.refreshMu.Lock()
	if ctx.Err() != nil || m.refreshRunning {
		m.refreshMu.Unlock()
		return
	}
	if err := m.ensureDocker(); err != nil || m.Docker == nil {
		m.refreshMu.Unlock()
		return
	}
	m.loadImageRefreshStateLocked()
	if !m.imageRefreshDueLocked(target) {
		m.refreshMu.Unlock()
		return
	}
	m.refreshRunning = true
	m.refreshMu.Unlock()

	defer func() {
		m.refreshMu.Lock()
		m.refreshRunning = false
		m.refreshMu.Unlock()
	}()

	busyLeft, pullFailed, incomplete := m.refreshRunnerImages(ctx)
	m.refreshMu.Lock()
	m.imageRefreshBusy = busyLeft
	if pullFailed {
		m.imageRefreshCheckedAtLocked(target, time.Now().Add(-m.imageRefreshEvery+runnerImageRefreshRetry))
	} else if !busyLeft && !incomplete {
		m.imageRefreshCheckedAtLocked(target, time.Now())
	}
	m.refreshMu.Unlock()
}

func (m *Manager) imageRefreshDueLocked(target string) bool {
	if m.imageRefreshEvery <= 0 {
		return false
	}
	if m.imageRefreshBusy {
		return true
	}
	if docker.IsLocalOnlyImage(target) {
		return false
	}
	at, ok := m.imageRefreshChecked[target]
	if !ok {
		return true
	}
	return time.Since(at) >= m.imageRefreshEvery
}

func (m *Manager) refreshRunnerImages(ctx context.Context) (busyLeft bool, pullFailed bool, incomplete bool) {
	runners, err := m.Store.List()
	if err != nil {
		slog.Warn("image-refresh: list store", "err", err)
		return false, true, false
	}
	target, _ := refreshImageRef(m.Image, "")
	slog.Info("image-refresh: checking fleet", "image", target, "runners", len(runners))
	var rebuilt int
	for _, rec := range runners {
		if ctx.Err() != nil {
			return busyLeft, true, true
		}
		info, err := m.inspectContainer(ctx, rec.ContainerName)
		if err != nil {
			slog.Warn("image-refresh: inspect", "runner", rec.Name, "id", rec.ID, "err", err)
			incomplete = true
			continue
		}
		if !info.Exists {
			continue
		}
		unlock := m.lockRunner(rec.ID)
		did, recErr := m.rebuildRunnerForCurrentImage(ctx, rec, info, false)
		unlock()
		if recErr != nil {
			if errors.Is(recErr, ErrRunnerBusy) {
				busyLeft = true
				continue
			}
			if errors.Is(recErr, docker.ErrUsingLocalImage) || errors.Is(recErr, docker.ErrImagePull) || errors.Is(recErr, ErrImagePull) {
				pullFailed = true
			}
			slog.Warn("image-refresh: rebuild failed", "runner", rec.Name, "id", rec.ID, "err", recErr)
			continue
		}
		if did {
			rebuilt++
		}
	}
	slog.Info("image-refresh: fleet check done", "image", target, "rebuilt", rebuilt, "waiting", busyLeft)
	return busyLeft, pullFailed, incomplete
}

// rebuildRunnerForCurrentImage recreates rec onto the current runner image when
// the container is behind. bringUp allows a stopped container to be rebuilt
// because the operator is starting it. The caller must hold the runner lock.
// A true result means the container was recreated and is already running.
func (m *Manager) rebuildRunnerForCurrentImage(ctx context.Context, rec store.Runner, info docker.InspectInfo, bringUp bool) (bool, error) {
	if m == nil || m.Docker == nil {
		return false, ErrDockerUnavailable
	}
	if !info.Exists {
		return false, nil
	}
	ref, fleet := refreshImageRef(m.Image, rec.Image)
	if docker.IsLocalOnlyImage(ref) {
		return false, nil
	}
	running := info.Running || info.Status == "restarting"
	// Leave a stopped runner stopped. Start rebuilds it before the old agent runs.
	if !running && !bringUp {
		return false, nil
	}
	currentID, pullErr := m.ensureImageID(ctx, ref)
	if pullErr != nil && currentID == "" {
		return false, pullErr
	}
	stale := runnerImageStale(info.ImageID, currentID)
	retarget := fleet && imageRefChanged(rec.Image, ref)
	disabled := false
	if raw, readErr := m.readRunnerConfig(ctx, rec.VolumeName); readErr == nil {
		disabled = runnerAutoUpdateDisabled(raw)
	}
	plan := planRunnerRebuild(true, stale, retarget, disabled, m.PATConfigured())
	if !plan.Recreate {
		if plan.Retarget {
			if err := m.persistRunnerImage(rec.ID, ref); err != nil {
				return false, err
			}
		}
		if plan.Warn {
			m.warnAutoUpdateOnce(rec)
		}
		if pullErr != nil {
			return false, pullErr
		}
		return false, nil
	}
	if running {
		busy, berr := m.runnerJobBusy(ctx, rec)
		if berr != nil {
			return false, berr
		}
		if busy {
			slog.Info("image-refresh: waiting for job to finish", "runner", rec.Name, "id", rec.ID)
			return false, fmt.Errorf("%w: job in progress", ErrRunnerBusy)
		}
	}
	fresh := rec
	if fleet {
		fresh.Image = ref
	}
	req := RecreateRequest{}
	log := runnerLog(fresh, "image-refresh")
	if plan.Reconfigure {
		tok, tokErr := m.resolveRegistrationToken(ctx, fresh.URL, "")
		if tokErr != nil {
			log.Warn("image-refresh: cannot reconfigure for auto-update", "err", tokErr)
			if !stale && !retarget {
				m.warnAutoUpdateOnce(fresh)
				return false, nil
			}
		} else {
			req.Token = tok
			req.ForceReconfigure = true
		}
	}
	if req.ForceReconfigure {
		log.Info("image-refresh: rebuilding with auto-update enabled", "image", ref)
	} else {
		log.Info("image-refresh: rebuilding onto current image", "image", ref)
		if plan.Warn {
			m.warnAutoUpdateOnce(fresh)
		}
	}
	if _, err := m.recreateRec(ctx, fresh, req, false, "image-refresh"); err != nil {
		return false, err
	}
	if fleet && imageRefChanged(rec.Image, ref) {
		if err := m.persistRunnerImage(rec.ID, ref); err != nil {
			log.Warn("image-refresh: store image", "err", err)
		}
	}
	return true, nil
}

func imageRefChanged(stored, target string) bool {
	stored = strings.TrimSpace(stored)
	target = strings.TrimSpace(target)
	if stored == "" || target == "" {
		return false
	}
	return stored != target
}

func (m *Manager) persistRunnerImage(id, image string) error {
	if m.Store == nil {
		return nil
	}
	fresh, err := m.Store.Get(id)
	if err != nil {
		return err
	}
	if strings.TrimSpace(fresh.Image) == image {
		return nil
	}
	fresh.Image = image
	return m.Store.Update(fresh)
}

// ensureImageID pulls ref when the cached pull is older than the refresh
// interval and returns the local image ID. A pull that falls back to a local
// image still returns that ID.
func (m *Manager) ensureImageID(ctx context.Context, ref string) (string, error) {
	m.refreshMu.Lock()
	if m.pulledImageIDs != nil && m.imagePulledAt != nil {
		if id := m.pulledImageIDs[ref]; looksLikeImageID(normalizeImageID(id)) {
			if at, ok := m.imagePulledAt[ref]; ok && time.Since(at) < m.imageRefreshEvery {
				m.refreshMu.Unlock()
				return id, nil
			}
		}
	}
	m.refreshMu.Unlock()

	err := m.Docker.EnsureImageCurrent(ctx, ref)
	hard := err != nil && !errors.Is(err, docker.ErrUsingLocalImage)
	if hard {
		m.noteImagePull(ref, time.Now().Add(-m.imageRefreshEvery+runnerImageRefreshRetry), "")
		return "", err
	}
	id, idErr := m.Docker.LocalImageID(ctx, ref)
	if idErr != nil || !looksLikeImageID(normalizeImageID(id)) {
		m.noteImagePull(ref, time.Now().Add(-m.imageRefreshEvery+runnerImageRefreshRetry), "")
		if idErr != nil {
			return "", idErr
		}
		return "", fmt.Errorf("%w: %s has no image id", docker.ErrImageNotFound, ref)
	}
	pulledAt := time.Now()
	if errors.Is(err, docker.ErrUsingLocalImage) {
		pulledAt = time.Now().Add(-m.imageRefreshEvery + runnerImageRefreshRetry)
	}
	m.noteImagePull(ref, pulledAt, id)
	return id, nil
}

func (m *Manager) noteImagePull(ref string, at time.Time, id string) {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	if m.imagePulledAt == nil {
		m.imagePulledAt = map[string]time.Time{}
	}
	m.imagePulledAt[ref] = at
	if id != "" {
		if m.pulledImageIDs == nil {
			m.pulledImageIDs = map[string]string{}
		}
		m.pulledImageIDs[ref] = id
	}
}

func (m *Manager) imageRefreshStatePath() string {
	if m == nil || m.Store == nil || m.Store.Path() == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(m.Store.Path()), "image-refresh.json")
}

// loadImageRefreshStateLocked reads the last completed check. The caller holds refreshMu.
func (m *Manager) loadImageRefreshStateLocked() {
	if m.imageRefreshLoaded {
		return
	}
	m.imageRefreshLoaded = true
	path := m.imageRefreshStatePath()
	if path == "" {
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var disk struct {
		Checked map[string]time.Time `json:"checked"`
	}
	if json.Unmarshal(b, &disk) != nil || len(disk.Checked) == 0 {
		return
	}
	m.imageRefreshChecked = disk.Checked
}

// imageRefreshCheckedAtLocked records when target was last checked. The caller holds refreshMu.
func (m *Manager) imageRefreshCheckedAtLocked(ref string, at time.Time) {
	if m.imageRefreshChecked == nil {
		m.imageRefreshChecked = map[string]time.Time{}
	}
	m.imageRefreshChecked[ref] = at
	m.saveImageRefreshStateLocked()
}

func (m *Manager) saveImageRefreshStateLocked() {
	path := m.imageRefreshStatePath()
	if path == "" || len(m.imageRefreshChecked) == 0 {
		return
	}
	b, err := json.Marshal(struct {
		Checked map[string]time.Time `json:"checked"`
	}{Checked: m.imageRefreshChecked})
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		slog.Warn("image-refresh: save state", "err", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		slog.Warn("image-refresh: save state", "err", err)
	}
}
