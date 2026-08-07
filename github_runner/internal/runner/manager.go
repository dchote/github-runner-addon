package runner

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/dchote/github-runner-addon/internal/container/docker"
	"github.com/dchote/github-runner-addon/internal/github"
	"github.com/dchote/github-runner-addon/internal/store"
	"github.com/google/uuid"
)

const (
	configFilesDir     = "/runner/data"
	DefaultRunnerImage = "myoung34/github-runner:latest"
)

var managedEnvKeys = map[string]struct{}{
	"RUNNER_NAME":                         {},
	"RUNNER_TOKEN":                        {},
	"LABELS":                              {},
	"RUNNER_SCOPE":                        {},
	"CONFIGURED_ACTIONS_RUNNER_FILES_DIR": {},
	"RUNNER_WORKDIR":                      {},
	"DISABLE_AUTO_UPDATE":                 {},
	"DISABLE_AUTOMATIC_DEREGISTRATION":    {},
	"ORG_NAME":                            {},
	"REPO_URL":                            {},
	"ACCESS_TOKEN":                        {},
	"APP_ID":                              {},
	"APP_PRIVATE_KEY":                     {},
	"APP_INSTALLATION_ID":                 {},
}

var registrationSuccessMarkers = []string{
	"Listening for Jobs",
	"Connected to GitHub",
	"Runner successfully added",
	"√ Connected to GitHub",
	"√ Listening for Jobs",
}

var registrationFailureMarkers = []string{
	"Invalid configuration",
	"401 Unauthorized",
	"403 Forbidden",
	"must be a valid",
	"Error: Http status code: 401",
	"Error: Http status code: 403",
	"Failed to configure",
}

// Manager orchestrates expected config + Docker lifecycle.
type Manager struct {
	Store           *store.Store
	Docker          *docker.Client
	GitHub          *github.Client
	Image           string
	MountDockerSock bool
	Version         string

	createLimiter *rateLimiter
	reconcileMu   sync.Mutex
	orphansMu     sync.RWMutex
	orphans       []OrphanView
}

type OrphanView struct {
	ContainerName string `json:"container_name"`
	RunnerID      string `json:"runner_id,omitempty"`
	Status        string `json:"status"`
	Running       bool   `json:"running"`
}

func NewManager(st *store.Store, d *docker.Client, gh *github.Client, image string, mountSock bool, version string) *Manager {
	if image == "" {
		image = DefaultRunnerImage
	}
	return &Manager{
		Store:           st,
		Docker:          d,
		GitHub:          gh,
		Image:           image,
		MountDockerSock: mountSock,
		Version:         version,
		createLimiter:   newRateLimiter(10, time.Minute),
	}
}

type CreateRequest struct {
	Name            string            `json:"name"`
	URL             string            `json:"url"`
	Token           string            `json:"token"`
	Labels          []string          `json:"labels"`
	Image           string            `json:"image,omitempty"`
	CPULimit        float64           `json:"cpu_limit,omitempty"`
	MemoryLimitMB   int64             `json:"memory_limit_mb,omitempty"`
	ExtraEnv        map[string]string `json:"extra_env,omitempty"`
	NetworkMode     string            `json:"network_mode,omitempty"`
	MountDockerSock *bool             `json:"mount_docker_sock,omitempty"`
}

type PatchRequest struct {
	Labels               []string          `json:"labels"`
	CPULimit             *float64          `json:"cpu_limit,omitempty"`
	MemoryLimitMB        *int64            `json:"memory_limit_mb,omitempty"`
	ExtraEnv             map[string]string `json:"extra_env,omitempty"`
	NetworkMode          *string           `json:"network_mode,omitempty"`
	MountDockerSock      *bool             `json:"mount_docker_sock,omitempty"`
	ResetMountDockerSock bool              `json:"reset_mount_docker_sock,omitempty"` // clear per-runner override
	Image                *string           `json:"image,omitempty"`
	Apply                bool              `json:"apply"`           // if true, recreate container to apply
	Token                string            `json:"token,omitempty"` // optional when apply=true
}

type RecreateRequest struct {
	Token string `json:"token"`
}

type View struct {
	store.Runner
	Status  string `json:"status"`
	Running bool   `json:"running"`
}

func (m *Manager) PATConfigured() bool {
	return m.GitHub != nil && m.GitHub.Configured()
}

func (m *Manager) enrich(ctx context.Context, r store.Runner) View {
	v := View{Runner: r, Status: "unknown"}
	if m.Docker == nil {
		return v
	}
	info, err := m.Docker.InspectByName(ctx, r.ContainerName)
	if err != nil {
		slog.Warn("docker inspect failed", "container", r.ContainerName, "err", err)
		return v
	}
	v.Status, v.Running = normalizeStatus(info)
	return v
}

func normalizeStatus(info docker.InspectInfo) (status string, running bool) {
	if !info.Exists {
		return "missing", false
	}
	switch info.Status {
	case "running", "restarting":
		return "running", true
	case "exited", "dead", "created", "paused", "removing":
		return "exited", false
	default:
		if info.Running {
			return "running", true
		}
		return "unknown", false
	}
}

func (m *Manager) List(ctx context.Context) ([]View, error) {
	runners, err := m.Store.List()
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(runners))
	for _, r := range runners {
		out = append(out, m.enrich(ctx, r))
	}
	return out, nil
}

// CountByStatus aggregates statuses from an already-enriched list (no Docker calls).
func CountByStatus(list []View) map[string]int {
	counts := map[string]int{"running": 0, "exited": 0, "missing": 0, "unknown": 0, "total": len(list)}
	for _, v := range list {
		counts[v.Status]++
	}
	return counts
}

func (m *Manager) StatusCounts(ctx context.Context) (map[string]int, error) {
	list, err := m.List(ctx)
	if err != nil {
		return nil, err
	}
	return CountByStatus(list), nil
}

func (m *Manager) Get(ctx context.Context, id string) (View, error) {
	r, err := m.Store.Get(id)
	if err != nil {
		return View{}, err
	}
	return m.enrich(ctx, r), nil
}

func (m *Manager) Orphans() []OrphanView {
	m.orphansMu.RLock()
	defer m.orphansMu.RUnlock()
	out := make([]OrphanView, len(m.orphans))
	copy(out, m.orphans)
	return out
}

func (m *Manager) parseProject(raw string) (github.ScopeInfo, error) {
	info, err := github.ParseProjectURL(raw)
	if err != nil {
		return github.ScopeInfo{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return info, nil
}

func (m *Manager) resolveRegistrationToken(ctx context.Context, projectURL, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token != "" {
		return token, nil
	}
	if !m.PATConfigured() {
		return "", fmt.Errorf("%w: registration token is required when GITHUB_PAT is not configured", ErrValidation)
	}
	minted, err := m.GitHub.MintRegistrationToken(ctx, projectURL)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrGitHub, sanitizeErr(err))
	}
	return minted, nil
}

func (m *Manager) Create(ctx context.Context, req CreateRequest) (View, error) {
	if m.Docker == nil {
		return View{}, ErrDockerUnavailable
	}
	if !m.createLimiter.Allow() {
		return View{}, fmt.Errorf("%w: too many create/recreate requests", ErrRateLimited)
	}

	name := strings.TrimSpace(req.Name)
	rawURL := strings.TrimSpace(req.URL)
	if name == "" || rawURL == "" {
		return View{}, fmt.Errorf("%w: name and url are required", ErrValidation)
	}
	info, err := m.parseProject(rawURL)
	if err != nil {
		return View{}, err
	}
	token, err := m.resolveRegistrationToken(ctx, info.HTMLURL, req.Token)
	if err != nil {
		return View{}, err
	}
	labels := normalizeLabels(req.Labels)
	if err := validateExtraEnv(req.ExtraEnv); err != nil {
		return View{}, err
	}
	id := uuid.NewString()
	norm := docker.NormalizeName(name)
	containerName := "gha-runner-" + norm
	volumeName := containerName + "-data"
	image := strings.TrimSpace(req.Image)
	if image == "" {
		image = m.Image
	}
	if image == "" {
		image = DefaultRunnerImage
	}

	now := time.Now().UTC()
	rec := store.Runner{
		ID:              id,
		Name:            name,
		URL:             info.HTMLURL,
		Scope:           info.Scope,
		Labels:          labels,
		ContainerName:   containerName,
		VolumeName:      volumeName,
		Image:           image,
		CreatedAt:       now,
		UpdatedAt:       now,
		CPULimit:        req.CPULimit,
		MemoryLimitMB:   req.MemoryLimitMB,
		ExtraEnv:        req.ExtraEnv,
		NetworkMode:     strings.TrimSpace(req.NetworkMode),
		MountDockerSock: req.MountDockerSock,
	}
	if err := m.Store.Add(rec); err != nil {
		return View{}, err
	}

	if err := m.startContainer(ctx, rec, token, info.OrgName(), true); err != nil {
		return m.rollbackFailedCreate(rec, err)
	}
	return m.Get(ctx, id)
}

// rollbackFailedCreate cleans up after a failed create. Uses a detached Docker
// context so request cancel/timeout cannot leave an orphaned container. If the
// container is already present with our managed labels (create raced and
// succeeded), keep the store row and treat the runner as created.
func (m *Manager) rollbackFailedCreate(rec store.Runner, createErr error) (View, error) {
	dctx, cancel := docker.DetachedContext()
	defer cancel()

	info, inspErr := m.Docker.InspectByName(dctx, rec.ContainerName)
	if inspErr == nil && info.Exists && info.Labels[docker.LabelID] == rec.ID {
		if !info.Running {
			if startErr := m.Docker.Start(dctx, rec.ContainerName); startErr != nil {
				slog.Warn("create rollback: container exists but start failed",
					"runner", rec.ID, "container", rec.ContainerName, "err", startErr)
			} else {
				info.Running = true
				info.Status = "running"
			}
		}
		slog.Warn("create reported failure but managed container exists; keeping runner",
			"runner", rec.ID, "container", rec.ContainerName, "status", info.Status, "err", createErr)
		return m.enrich(dctx, rec), nil
	}

	if remErr := m.Docker.Remove(dctx, rec.ContainerName, rec.VolumeName); remErr != nil {
		slog.Error("create rollback: remove failed; leaving store row for operator cleanup",
			"runner", rec.ID, "container", rec.ContainerName, "err", remErr)
		return View{}, fmt.Errorf("%w (cleanup failed: %v)", createErr, remErr)
	}
	_ = m.Store.Delete(rec.ID)
	return View{}, createErr
}

func (m *Manager) startContainer(ctx context.Context, rec store.Runner, token, orgName string, waitRegister bool) error {
	env := m.buildEnv(rec, token, orgName)
	mountSock := m.MountDockerSock
	if rec.MountDockerSock != nil {
		mountSock = *rec.MountDockerSock
	}
	_, err := m.Docker.CreateAndStart(ctx, docker.CreateOpts{
		Name:  rec.ContainerName,
		Image: rec.Image,
		Env:   env,
		Labels: map[string]string{
			docker.LabelManaged: "true",
			docker.LabelID:      rec.ID,
		},
		VolumeName:      rec.VolumeName,
		VolumeTarget:    configFilesDir,
		MountDockerSock: mountSock,
		RestartPolicy:   "unless-stopped",
		CPULimit:        rec.CPULimit,
		MemoryLimitMB:   rec.MemoryLimitMB,
		NetworkMode:     rec.NetworkMode,
	})
	if err != nil {
		if docker.IsConflict(err) {
			return fmt.Errorf("%w: container or volume name already exists", store.ErrConflict)
		}
		return fmt.Errorf("create container: %w", err)
	}
	if waitRegister && token != "" {
		confirmed, err := m.waitForRegistration(ctx, rec.ContainerName)
		if err != nil {
			return err
		}
		if confirmed {
			if scrubErr := m.scrubToken(ctx, rec, orgName); scrubErr != nil {
				slog.Warn("token scrub failed", "runner", rec.ID, "err", scrubErr)
			}
		} else {
			slog.Warn("registration not confirmed in logs; leaving RUNNER_TOKEN until next recreate", "runner", rec.ID)
		}
	}
	return nil
}

func (m *Manager) buildEnv(rec store.Runner, token, orgName string) []string {
	env := []string{
		"RUNNER_NAME=" + rec.Name,
		"LABELS=" + strings.Join(rec.Labels, ","),
		"RUNNER_SCOPE=" + rec.Scope,
		"CONFIGURED_ACTIONS_RUNNER_FILES_DIR=" + configFilesDir,
		"RUNNER_WORKDIR=/tmp/runner/work",
		"DISABLE_AUTO_UPDATE=true",
		"DISABLE_AUTOMATIC_DEREGISTRATION=true",
	}
	if token != "" {
		env = append(env, "RUNNER_TOKEN="+token)
	}
	if rec.Scope == "org" {
		if orgName == "" {
			if info, err := github.ParseProjectURL(rec.URL); err == nil {
				orgName = info.OrgName()
			}
		}
		env = append(env, "ORG_NAME="+orgName)
	} else {
		env = append(env, "REPO_URL="+rec.URL)
	}
	for k, v := range rec.ExtraEnv {
		env = append(env, k+"="+v)
	}
	return env
}

// waitForRegistration returns (confirmed, err). confirmed means success markers were seen
// (safe to scrub token). Soft timeout while still running returns (false, nil).
func (m *Manager) waitForRegistration(ctx context.Context, containerName string) (bool, error) {
	deadline := time.Now().Add(90 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			// Request canceled/timed out after the container started: do not fail
			// hard if the runner is still up — treat like a soft timeout.
			dctx, cancel := docker.DetachedContext()
			info, err := m.Docker.InspectByName(dctx, containerName)
			cancel()
			if err == nil && info.Exists && info.Running {
				slog.Warn("registration wait interrupted; container still running",
					"container", containerName, "err", ctx.Err())
				return false, nil
			}
			return false, ctx.Err()
		default:
		}
		info, err := m.Docker.InspectByName(ctx, containerName)
		if err == nil && info.Exists && !info.Running && info.Status == "exited" {
			logs, _ := m.Docker.TailLogs(ctx, containerName, "80")
			return false, fmt.Errorf("%w: runner container exited during registration: %s", ErrValidation, summarizeLogs(logs))
		}
		logs, err := m.Docker.TailLogs(ctx, containerName, "80")
		if err == nil {
			last = logs
			lower := strings.ToLower(logs)
			for _, marker := range registrationFailureMarkers {
				if strings.Contains(lower, strings.ToLower(marker)) {
					return false, fmt.Errorf("%w: registration failed: %s", ErrValidation, summarizeLogs(logs))
				}
			}
			for _, marker := range registrationSuccessMarkers {
				if strings.Contains(logs, marker) {
					return true, nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	info, err := m.Docker.InspectByName(ctx, containerName)
	if err == nil && info.Exists && info.Running {
		slog.Warn("registration wait timed out without success markers; container still running", "container", containerName, "tail", summarizeLogs(last))
		return false, nil
	}
	if last != "" {
		return false, fmt.Errorf("%w: timed out waiting for GitHub registration: %s", ErrValidation, summarizeLogs(last))
	}
	return false, fmt.Errorf("%w: timed out waiting for GitHub registration", ErrValidation)
}

func summarizeLogs(logs string) string {
	logs = strings.TrimSpace(logs)
	if logs == "" {
		return "(no logs)"
	}
	lines := strings.Split(logs, "\n")
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	msg := strings.Join(lines, " | ")
	if len(msg) > 400 {
		return msg[len(msg)-400:]
	}
	return msg
}

// scrubToken recreates the container without RUNNER_TOKEN so docker inspect cannot retain it.
func (m *Manager) scrubToken(ctx context.Context, rec store.Runner, orgName string) error {
	if err := m.Docker.RemoveContainer(ctx, rec.ContainerName); err != nil {
		return err
	}
	if err := m.startContainer(ctx, rec, "", orgName, false); err != nil {
		return err
	}
	info, err := m.Docker.InspectByName(ctx, rec.ContainerName)
	if err != nil {
		return err
	}
	if docker.EnvHasKey(info.Env, "RUNNER_TOKEN") {
		return fmt.Errorf("RUNNER_TOKEN still present after scrub")
	}
	return nil
}

func (m *Manager) Patch(ctx context.Context, id string, req PatchRequest) (View, error) {
	rec, err := m.Store.Get(id)
	if err != nil {
		return View{}, err
	}
	if req.Labels != nil {
		rec.Labels = normalizeLabels(req.Labels)
	}
	if req.CPULimit != nil {
		rec.CPULimit = *req.CPULimit
	}
	if req.MemoryLimitMB != nil {
		rec.MemoryLimitMB = *req.MemoryLimitMB
	}
	if req.ExtraEnv != nil {
		if err := validateExtraEnv(req.ExtraEnv); err != nil {
			return View{}, err
		}
		rec.ExtraEnv = req.ExtraEnv
	}
	if req.NetworkMode != nil {
		rec.NetworkMode = strings.TrimSpace(*req.NetworkMode)
	}
	if req.ResetMountDockerSock {
		rec.MountDockerSock = nil
	} else if req.MountDockerSock != nil {
		rec.MountDockerSock = req.MountDockerSock
	}
	if req.Image != nil {
		img := strings.TrimSpace(*req.Image)
		if img == "" {
			img = m.Image
		}
		rec.Image = img
	}
	if err := m.Store.Update(rec); err != nil {
		return View{}, err
	}
	if req.Apply {
		return m.Recreate(ctx, id, RecreateRequest{Token: req.Token})
	}
	return m.Get(ctx, id)
}

func (m *Manager) Recreate(ctx context.Context, id string, req RecreateRequest) (View, error) {
	if m.Docker == nil {
		return View{}, ErrDockerUnavailable
	}
	if !m.createLimiter.Allow() {
		return View{}, fmt.Errorf("%w: too many create/recreate requests", ErrRateLimited)
	}
	rec, err := m.Store.Get(id)
	if err != nil {
		return View{}, err
	}
	info, err := m.parseProject(rec.URL)
	if err != nil {
		return View{}, err
	}

	containerInfo, _ := m.Docker.InspectByName(ctx, rec.ContainerName)
	volExists, volErr := m.Docker.VolumeExists(ctx, rec.VolumeName)
	if volErr != nil {
		slog.Warn("volume inspect failed", "volume", rec.VolumeName, "err", volErr)
	}
	// Registration files live on the volume. Reuse when the volume exists (even if the
	// container is missing). Fresh registration is required when there is no volume.
	canReuseRegistration := volExists
	needsToken := !canReuseRegistration

	token := strings.TrimSpace(req.Token)
	if needsToken {
		var resolveErr error
		token, resolveErr = m.resolveRegistrationToken(ctx, rec.URL, token)
		if resolveErr != nil {
			return View{}, fmt.Errorf("%w: recreate requires a registration token or GITHUB_PAT when the runner data volume is missing", ErrValidation)
		}
	} else if token == "" && m.PATConfigured() {
		// Optional refresh of registration when operator has PAT (covers wiped volume contents).
		if minted, err := m.GitHub.MintRegistrationToken(ctx, rec.URL); err != nil {
			slog.Warn("recreate: optional mint failed; reusing volume registration", "err", err)
		} else {
			token = minted
		}
	}

	if containerInfo.Exists {
		if err := m.Docker.RemoveContainer(ctx, rec.ContainerName); err != nil && !docker.IsNotFound(err) {
			return View{}, fmt.Errorf("remove container: %w", err)
		}
	}
	wait := token != ""
	if err := m.startContainer(ctx, rec, token, info.OrgName(), wait); err != nil {
		return View{}, err
	}
	return m.Get(ctx, id)
}

func (m *Manager) applyLifecycle(ctx context.Context, id string, op func(context.Context, string) error) (View, error) {
	if m.Docker == nil {
		return View{}, ErrDockerUnavailable
	}
	r, err := m.Store.Get(id)
	if err != nil {
		return View{}, err
	}
	if err := op(ctx, r.ContainerName); err != nil {
		return View{}, err
	}
	return m.Get(ctx, id)
}

func (m *Manager) Start(ctx context.Context, id string) (View, error) {
	return m.applyLifecycle(ctx, id, func(ctx context.Context, name string) error {
		return m.Docker.Start(ctx, name)
	})
}

func (m *Manager) Stop(ctx context.Context, id string) (View, error) {
	return m.applyLifecycle(ctx, id, func(ctx context.Context, name string) error {
		return m.Docker.Stop(ctx, name)
	})
}

func (m *Manager) Restart(ctx context.Context, id string) (View, error) {
	return m.applyLifecycle(ctx, id, func(ctx context.Context, name string) error {
		return m.Docker.Restart(ctx, name)
	})
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	r, err := m.Store.Get(id)
	if err != nil {
		return err
	}
	if m.Docker == nil {
		return ErrDockerUnavailable
	}
	if m.PATConfigured() {
		if err := m.GitHub.DeregisterRunner(ctx, r.URL, r.Name); err != nil {
			slog.Warn("github deregister failed; continuing local delete", "runner", r.Name, "err", err)
		}
	}
	if err := m.Docker.Remove(ctx, r.ContainerName, r.VolumeName); err != nil {
		if !docker.IsNotFound(err) {
			return err
		}
	}
	return m.Store.Delete(id)
}

func (m *Manager) Logs(ctx context.Context, id string, follow bool, tail string) (io.ReadCloser, error) {
	r, err := m.Store.Get(id)
	if err != nil {
		return nil, err
	}
	if m.Docker == nil {
		return nil, ErrDockerUnavailable
	}
	return m.Docker.Logs(ctx, r.ContainerName, follow, tail)
}

// Reconcile syncs Docker managed containers with the store and records orphans.
// Missing containers are not written to the store — List/Get report status=missing
// via live Docker inspect. Orphans are managed-labeled containers with no store row.
func (m *Manager) Reconcile(ctx context.Context) {
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	if m.Docker == nil {
		return
	}
	runners, err := m.Store.List()
	if err != nil {
		slog.Warn("reconcile: list store", "err", err)
		return
	}
	byContainer := make(map[string]store.Runner, len(runners))
	byID := make(map[string]store.Runner, len(runners))
	for _, r := range runners {
		byContainer[r.ContainerName] = r
		byID[r.ID] = r
	}
	managed, err := m.Docker.ListManaged(ctx)
	if err != nil {
		slog.Warn("reconcile: list managed", "err", err)
		return
	}
	var orphans []OrphanView
	for _, c := range managed {
		if _, ok := byContainer[c.Name]; ok {
			continue
		}
		if c.RunnerID != "" {
			if _, ok := byID[c.RunnerID]; ok {
				continue
			}
		}
		orphans = append(orphans, OrphanView{
			ContainerName: c.Name,
			RunnerID:      c.RunnerID,
			Status:        c.Status,
			Running:       c.Running,
		})
	}
	m.orphansMu.Lock()
	m.orphans = orphans
	m.orphansMu.Unlock()
	if len(orphans) > 0 {
		slog.Info("reconcile: orphan managed containers", "count", len(orphans))
	}
}

// StartReconcileLoop runs Reconcile immediately and on an interval until ctx is done.
func (m *Manager) StartReconcileLoop(ctx context.Context, every time.Duration) {
	m.Reconcile(ctx)
	if every <= 0 {
		every = 2 * time.Minute
	}
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.Reconcile(ctx)
			}
		}
	}()
}

func normalizeLabels(labels []string) []string {
	if len(labels) == 0 {
		return []string{"self-hosted", "linux"}
	}
	out := make([]string, 0, len(labels))
	for _, l := range labels {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return []string{"self-hosted", "linux"}
	}
	return out
}

func validateExtraEnv(env map[string]string) error {
	for k := range env {
		k = strings.TrimSpace(k)
		if k == "" {
			return fmt.Errorf("%w: empty extra_env key", ErrValidation)
		}
		if _, blocked := managedEnvKeys[k]; blocked {
			return fmt.Errorf("%w: extra_env key %q is reserved", ErrValidation, k)
		}
		if strings.ContainsAny(k, "=\n\r") {
			return fmt.Errorf("%w: invalid extra_env key", ErrValidation)
		}
	}
	return nil
}

func sanitizeErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "Bearer ", "Bearer ***")
	if i := strings.Index(strings.ToLower(msg), "ghp_"); i >= 0 {
		msg = msg[:i] + "ghp_***"
	}
	if i := strings.Index(strings.ToLower(msg), "github_pat_"); i >= 0 {
		msg = msg[:i] + "github_pat_***"
	}
	return fmt.Errorf("%s", msg)
}

type rateLimiter struct {
	mu     sync.Mutex
	events []time.Time
	max    int
	window time.Duration
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{max: max, window: window}
}

func (r *rateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	cut := now.Add(-r.window)
	kept := r.events[:0]
	for _, t := range r.events {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	r.events = kept
	if len(r.events) >= r.max {
		return false
	}
	r.events = append(r.events, now)
	return true
}
