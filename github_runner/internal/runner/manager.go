package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/dchote/github-runner-addon/internal/container/docker"
	"github.com/dchote/github-runner-addon/internal/github"
	"github.com/dchote/github-runner-addon/internal/store"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

const (
	configFilesDir     = "/runner/data"
	DefaultRunnerImage = "myoung34/github-runner:latest"

	// lifecycleOpTimeout bounds Create/Recreate Docker work on a detached context so
	// client disconnect / ingress cancel cannot abort mid-pipeline (or after clearing .runner).
	lifecycleOpTimeout = 9 * time.Minute
	// detachedOpTimeout bounds shorter detached Docker ops (rollback, configure container swap).
	detachedOpTimeout = 3 * time.Minute

	// listEnrichConcurrency bounds parallel enrich workers during List.
	listEnrichConcurrency = 8

	// configureNoopCmd is the Docker CMD override for the configure phase.
	// myoung34's entrypoint runs config.sh then execs CMD; `true` exits without
	// starting the listener (DEBUG_ONLY skips config.sh when .runner is missing).
	configureNoopCmd = "true"

	// detachedLifecycleTimeout bounds Stop/Delete/Restart Docker work so ingress
	// cancel cannot abort mid-remove (stop grace 120s plus round-trips).
	detachedLifecycleTimeout = 3 * time.Minute

	// maxCPULimit / maxMemoryLimitMB reject unbounded resource claims.
	maxCPULimit      = 256.0
	maxMemoryLimitMB = 1024 * 1024 // 1 TiB in MiB
)

func lifecycleContext() (context.Context, context.CancelFunc) {
	return docker.DetachedTimeout(lifecycleOpTimeout)
}

var managedEnvKeys = map[string]struct{}{
	"RUNNER_NAME":                         {},
	"RUNNER_TOKEN":                        {},
	"LABELS":                              {},
	"RUNNER_SCOPE":                        {},
	"CONFIGURED_ACTIONS_RUNNER_FILES_DIR": {},
	"RUNNER_WORKDIR":                      {},
	"RUNNER_CACHE":                        {},
	"DISABLE_AUTO_UPDATE":                 {},
	"DISABLE_AUTOMATIC_DEREGISTRATION":    {},
	"DEBUG_ONLY":                          {},
	"ORG_NAME":                            {},
	"REPO_URL":                            {},
	"ACCESS_TOKEN":                        {},
	"APP_ID":                              {},
	"APP_PRIVATE_KEY":                     {},
	"APP_INSTALLATION_ID":                 {},
	"ACTIONS_RUNNER_HOOK_JOB_STARTED":     {},
	"ACTIONS_RUNNER_HOOK_JOB_COMPLETED":   {},
}

// configureSuccessMarkers indicate myoung34 config.sh finished.
// Prefer confirming .runner on the volume; these are secondary log signals.
var configureSuccessMarkers = []string{
	"Settings Saved",
	"Runner successfully added",
	"√ Settings Saved",
}

// listenSuccessMarkers indicate the runner listener is online (tokenless run phase).
// Do not treat "Connected to GitHub" alone as success — that can precede a session conflict.
var listenSuccessMarkers = []string{
	"Listening for Jobs",
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
	"A session for this runner already exists",
	"SessionConflictException",
	"Stop retry on SessionConflictException",
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

	dockerMu      sync.Mutex
	dockerFactory func() (*docker.Client, error)

	// workdirHost overrides Docker for EnsureHostDir / volume file ops (tests).
	workdirHost workdirHost

	// agentWorkFolder caches successful .runner reads (and missing-file) per data volume.
	agentWFMu sync.RWMutex
	agentWF   map[string]agentWorkFolderCache

	// jobStatusCache short-TTL cache of status.json bytes (keyed by host path).
	jobStatusMu    sync.RWMutex
	jobStatusCache map[string]jobStatusCacheEntry

	// runnerLocks serializes Create (create:<containerName> + id), Patch, Recreate,
	// Delete, Start, Stop, Restart per key.
	runnerLocks sync.Map // string -> *sync.Mutex

	// verifyTimeout bounds post-start workFolder checks (0 = 45s). Tests may shorten it.
	verifyTimeout time.Duration

	// inspectFn / createAndStartFn / readContainerFileFn are test seams (nil → Docker client).
	inspectFn           func(ctx context.Context, name string) (docker.InspectInfo, error)
	createAndStartFn    func(ctx context.Context, opts docker.CreateOpts) (string, error)
	readContainerFileFn func(ctx context.Context, name, path string) ([]byte, error)

	// listenTimeout bounds waitForListening (0 = 90s). Tests may shorten it.
	listenTimeout time.Duration
}

// errInspectUnavailable is returned when neither Docker nor inspectFn can inspect.
var errInspectUnavailable = errors.New("docker inspect unavailable")

// errContainerReadUnavailable is returned when neither Docker nor readContainerFileFn can copy files.
var errContainerReadUnavailable = errors.New("container file read unavailable")

// lockRunner returns an unlock func; callers must defer unlock().
// Prefer lockRunnerOpt(..., true) for create:<name> and Delete so idle keys are pruned.
func (m *Manager) lockRunner(key string) func() {
	return m.lockRunnerOpt(key, false)
}

func (m *Manager) lockRunnerOpt(key string, forget bool) func() {
	v, _ := m.runnerLocks.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return func() {
		mu.Unlock()
		if !forget {
			return
		}
		// Only forget if nobody else grabbed the lock between Unlock and TryLock.
		if mu.TryLock() {
			m.runnerLocks.Delete(key)
			mu.Unlock()
		}
	}
}

type agentWorkFolderCache struct {
	folder  string
	missing bool // true when .runner was confirmed absent
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
		agentWF:         make(map[string]agentWorkFolderCache),
	}
}

// SetDockerFactory allows reconnecting when the engine was unavailable at boot.
func (m *Manager) SetDockerFactory(fn func() (*docker.Client, error)) {
	m.dockerFactory = fn
}

// EnsureDocker connects via dockerFactory when the client is currently nil.
func (m *Manager) EnsureDocker() error {
	return m.ensureDocker()
}

func (m *Manager) ensureDocker() error {
	if m == nil {
		return ErrDockerUnavailable
	}
	m.dockerMu.Lock()
	defer m.dockerMu.Unlock()
	if m.Docker != nil {
		return nil
	}
	if m.dockerFactory == nil {
		return ErrDockerUnavailable
	}
	c, err := m.dockerFactory()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDockerUnavailable, err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := c.Ping(pingCtx); err != nil {
		_ = c.Close()
		return fmt.Errorf("%w: %w", ErrDockerUnavailable, err)
	}
	m.Docker = c
	slog.Info("docker client connected")
	return nil
}

func (m *Manager) requireDocker() error {
	if m.Docker != nil {
		return nil
	}
	if err := m.ensureDocker(); err != nil {
		return err
	}
	if m.Docker == nil {
		return ErrDockerUnavailable
	}
	return nil
}

type CreateRequest struct {
	Name            string             `json:"name"`
	URL             string             `json:"url"`
	Token           string             `json:"token"`
	Labels          []string           `json:"labels"`
	Image           string             `json:"image,omitempty"`
	CPULimit        float64            `json:"cpu_limit,omitempty"`
	MemoryLimitMB   int64              `json:"memory_limit_mb,omitempty"`
	ExtraEnv        map[string]string  `json:"extra_env,omitempty"`
	NetworkMode     string             `json:"network_mode,omitempty"`
	MountDockerSock *bool              `json:"mount_docker_sock,omitempty"`
	Cache           *store.CacheConfig `json:"cache,omitempty"`
	WorkdirHostPath string             `json:"workdir_host_path,omitempty"`
}

type PatchRequest struct {
	Labels               []string           `json:"labels"`
	CPULimit             *float64           `json:"cpu_limit,omitempty"`
	MemoryLimitMB        *int64             `json:"memory_limit_mb,omitempty"`
	ExtraEnv             map[string]string  `json:"extra_env,omitempty"`
	NetworkMode          *string            `json:"network_mode,omitempty"`
	MountDockerSock      *bool              `json:"mount_docker_sock,omitempty"`
	ResetMountDockerSock bool               `json:"reset_mount_docker_sock,omitempty"` // clear per-runner override
	Image                *string            `json:"image,omitempty"`
	Cache                *store.CacheConfig `json:"cache,omitempty"`
	WorkdirHostPath      *string            `json:"workdir_host_path,omitempty"` // set "" to reset to default
	Apply                bool               `json:"apply"`                       // if true, recreate container to apply
	Token                string             `json:"token,omitempty"`             // optional when apply=true
}

type RecreateRequest struct {
	Token string `json:"token"`
}

type RecreateMissingRequest struct {
	Token string `json:"token"`
}

type RecreateMissingFailure struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

type RecreateMissingResult struct {
	Recreated []string                 `json:"recreated"`
	Failed    []RecreateMissingFailure `json:"failed"`
}

type View struct {
	store.Runner
	Status           string      `json:"status"`
	Running          bool        `json:"running"`
	JobState         string      `json:"job_state,omitempty"`         // idle|busy|unknown when Running
	CurrentJob       *CurrentJob `json:"current_job,omitempty"`       // set when job_state=busy
	WorkdirEffective string      `json:"workdir_effective,omitempty"` // resolved host path (RUNNER_WORKDIR / bind)
	WorkdirAgent     string      `json:"workdir_agent,omitempty"`     // workFolder from /runner/data/.runner
	WorkdirMismatch  bool        `json:"workdir_mismatch,omitempty"`  // agent workFolder ≠ effective path
	WorkdirError     string      `json:"workdir_error,omitempty"`     // diagnostics error reading .runner
	CacheEffective   string      `json:"cache_effective,omitempty"`   // resolved cache path (RUNNER_CACHE)
	Warnings         []string    `json:"warnings,omitempty"`          // soft config advisories (do not fail the request)
}

func (m *Manager) PATConfigured() bool {
	return m.GitHub != nil && m.GitHub.Configured()
}

type enrichOpts struct {
	// workdirDiag requests a fresh .runner read. List uses cache-only.
	// Get live-reads via CopyFromContainer only while the container is running
	// (missing/exited stay cache-only — no alpine volume helpers).
	workdirDiag bool
	// jobStatusLive allows CopyFromContainer + host helper reads for status.json.
	// List keeps this false (cache + CopyFromContainer only) to avoid alpine helper storms.
	jobStatusLive bool
}

func (m *Manager) enrich(ctx context.Context, r store.Runner, opts enrichOpts) View {
	return m.enrichWithManaged(ctx, r, nil, false, opts)
}

// enrichWithManaged builds a View.
// When fromManagedList is true, mc is the batched ListManaged hit (or nil if absent) —
// InspectByName is skipped so a Docker-reset fleet does not N-inspect missing names.
func (m *Manager) enrichWithManaged(ctx context.Context, r store.Runner, mc *docker.ManagedContainer, fromManagedList bool, opts enrichOpts) View {
	// Coerce bind cache onto the View copy so API clients see same-path target
	// without requiring a store rewrite on every List.
	if r.Cache != nil && r.Cache.Enabled {
		r.Cache = normalizeCache(r.Cache)
	}
	v := View{
		Runner:           r,
		Status:           "unknown",
		WorkdirEffective: resolveWorkdirHostPath(r),
		CacheEffective:   resolveCacheEffective(r.Cache),
	}
	if warns := cacheSiblingWarnings(r.Cache); len(warns) > 0 {
		v.Warnings = append(v.Warnings, warns...)
	}
	info, ok := m.resolveInspect(ctx, r, mc, fromManagedList)
	if !ok {
		m.applyWorkdirDiagnostics(ctx, &v, false)
		return v
	}
	v.Status, v.Running = normalizeStatus(info)
	// Live .runner reads use CopyFromContainer — only while the container is running.
	m.applyWorkdirDiagnostics(ctx, &v, opts.workdirDiag && v.Running)
	if v.Running {
		ref := info.ID
		if ref == "" {
			ref = r.ContainerName
		}
		m.applyJobStatus(ctx, &v, ref, v.WorkdirEffective, opts.jobStatusLive)
	}
	return v
}

func (m *Manager) resolveInspect(ctx context.Context, r store.Runner, mc *docker.ManagedContainer, fromManagedList bool) (docker.InspectInfo, bool) {
	if mc != nil {
		return docker.InspectInfo{
			Name:    r.ContainerName,
			Exists:  true,
			ID:      mc.ID,
			Status:  mc.Status,
			Running: mc.Running,
		}, true
	}
	if fromManagedList {
		return docker.InspectInfo{Name: r.ContainerName, Exists: false}, true
	}
	info, err := m.inspectContainer(ctx, r.ContainerName)
	if err != nil {
		if !errors.Is(err, errInspectUnavailable) && !docker.IsContextError(err) {
			slog.Warn("docker inspect failed", "container", r.ContainerName, "err", err)
		}
		return docker.InspectInfo{}, false
	}
	return info, true
}

func (m *Manager) inspectContainer(ctx context.Context, name string) (docker.InspectInfo, error) {
	if m.inspectFn != nil {
		return m.inspectFn(ctx, name)
	}
	if m.Docker == nil {
		return docker.InspectInfo{Name: name}, errInspectUnavailable
	}
	return m.Docker.InspectByName(ctx, name)
}

func (m *Manager) applyWorkdirDiagnostics(ctx context.Context, v *View, live bool) {
	if v.VolumeName == "" {
		return
	}
	if live {
		wf, err := m.readAgentWorkFolderPreferContainer(ctx, v.Runner, true)
		if err != nil {
			if errors.Is(err, errNoRunnerConfig) {
				return
			}
			if docker.IsContextError(err) {
				return
			}
			v.WorkdirError = err.Error()
			slog.Warn("read .runner workFolder failed", "volume", v.VolumeName, "err", err)
			return
		}
		if wf != "" {
			v.WorkdirAgent = wf
			v.WorkdirMismatch = !workdirPathsMatch(wf, v.WorkdirEffective)
		}
		return
	}
	// List path: never spawn helper containers — use cache populated by Get/verify.
	m.agentWFMu.RLock()
	c, hit := m.agentWF[v.VolumeName]
	m.agentWFMu.RUnlock()
	if !hit || c.missing || c.folder == "" {
		return
	}
	v.WorkdirAgent = c.folder
	v.WorkdirMismatch = !workdirPathsMatch(c.folder, v.WorkdirEffective)
}

func (m *Manager) readAgentWorkFolder(ctx context.Context, volumeName string, bypassCache bool) (string, error) {
	if !bypassCache {
		m.agentWFMu.RLock()
		if c, hit := m.agentWF[volumeName]; hit {
			m.agentWFMu.RUnlock()
			if c.missing {
				return "", errNoRunnerConfig
			}
			return c.folder, nil
		}
		m.agentWFMu.RUnlock()
	}

	wh := m.workdirHostOrDocker()
	if wh == nil {
		return "", fmt.Errorf("docker unavailable")
	}
	raw, err := wh.ReadVolumeFile(ctx, volumeName, runnerConfigFile)
	if err != nil {
		if errors.Is(err, docker.ErrVolumeFileNotFound) {
			m.cacheAgentWorkFolderMissing(volumeName)
			return "", errNoRunnerConfig
		}
		// Do not cache hard errors — retry on next enrich/recreate.
		return "", err
	}
	wf, err := parseRunnerWorkFolder(raw)
	if err != nil {
		return "", err
	}
	m.cacheAgentWorkFolder(volumeName, wf)
	return wf, nil
}

func (m *Manager) cacheAgentWorkFolder(volumeName, folder string) {
	m.agentWFMu.Lock()
	if m.agentWF == nil {
		m.agentWF = make(map[string]agentWorkFolderCache)
	}
	m.agentWF[volumeName] = agentWorkFolderCache{folder: folder}
	m.agentWFMu.Unlock()
}

func (m *Manager) cacheAgentWorkFolderMissing(volumeName string) {
	m.agentWFMu.Lock()
	if m.agentWF == nil {
		m.agentWF = make(map[string]agentWorkFolderCache)
	}
	m.agentWF[volumeName] = agentWorkFolderCache{missing: true}
	m.agentWFMu.Unlock()
}

func (m *Manager) invalidateAgentWorkFolder(volumeName string) {
	if volumeName == "" {
		return
	}
	m.agentWFMu.Lock()
	delete(m.agentWF, volumeName)
	m.agentWFMu.Unlock()
}

// clearRunnerConfigForReconfigure removes .runner so myoung34 will run config.sh
// with the current RUNNER_WORKDIR (--work). Credentials on the volume are kept;
// a registration token or PAT is required for the reconfigure call.
func (m *Manager) clearRunnerConfigForReconfigure(ctx context.Context, volumeName string) error {
	if volumeName == "" {
		return nil
	}
	m.invalidateAgentWorkFolder(volumeName)
	wh := m.workdirHostOrDocker()
	if wh == nil {
		return fmt.Errorf("docker unavailable")
	}
	return wh.RemoveVolumeFiles(ctx, volumeName, runnerConfigFile)
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
	if len(runners) == 0 {
		return nil, nil
	}

	managedByName := map[string]docker.ManagedContainer{}
	fromManagedList := false
	if m.Docker != nil {
		if managed, err := m.Docker.ListManaged(ctx); err == nil {
			fromManagedList = true
			for _, c := range managed {
				if c.Name != "" {
					managedByName[c.Name] = c
				}
			}
		} else if !docker.IsContextError(err) {
			slog.Warn("list managed containers failed; falling back to per-runner inspect", "err", err)
		}
	}

	out := make([]View, len(runners))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(listEnrichConcurrency)
	for i, r := range runners {
		i, r := i, r
		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}
			var mc *docker.ManagedContainer
			if c, ok := managedByName[r.ContainerName]; ok {
				cp := c
				mc = &cp
			}
			out[i] = m.enrichWithManaged(gctx, r, mc, fromManagedList, enrichOpts{workdirDiag: false, jobStatusLive: false})
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		// Enrich itself does not return errors; treat unexpected group errors as fatal.
		return nil, err
	}
	return out, nil
}

// CountByStatus aggregates statuses from an already-enriched list (no Docker calls).
// Useful for tests and callers that already have Views; Health uses StatusCounts.
func CountByStatus(list []View) map[string]int {
	counts := map[string]int{"running": 0, "exited": 0, "missing": 0, "unknown": 0, "total": len(list)}
	for _, v := range list {
		counts[v.Status]++
	}
	return counts
}

// StatusCounts is a cheap health-path aggregator: one ListManaged + store rows,
// no job-status helpers and no per-runner InspectByName.
// When Docker is unavailable or ListManaged fails, statuses are "unknown"
// (same as List enrich when inspect is unavailable).
func (m *Manager) StatusCounts(ctx context.Context) (map[string]int, error) {
	runners, err := m.Store.List()
	if err != nil {
		return nil, err
	}
	counts := map[string]int{"running": 0, "exited": 0, "missing": 0, "unknown": 0, "total": len(runners)}
	if len(runners) == 0 {
		return counts, nil
	}
	if m.Docker == nil {
		counts["unknown"] = len(runners)
		return counts, nil
	}
	managed, err := m.Docker.ListManaged(ctx)
	if err != nil {
		if docker.IsContextError(err) {
			return counts, err
		}
		slog.Warn("status counts: list managed failed", "err", err)
		counts["unknown"] = len(runners)
		return counts, nil
	}
	byName := make(map[string]docker.ManagedContainer, len(managed))
	for _, c := range managed {
		if c.Name != "" {
			byName[c.Name] = c
		}
	}
	for _, r := range runners {
		c, ok := byName[r.ContainerName]
		if !ok {
			counts["missing"]++
			continue
		}
		st, _ := normalizeStatus(docker.InspectInfo{
			Exists:  true,
			Status:  c.Status,
			Running: c.Running,
		})
		counts[st]++
	}
	return counts, nil
}

func (m *Manager) Get(ctx context.Context, id string) (View, error) {
	r, err := m.Store.Get(id)
	if err != nil {
		return View{}, err
	}
	return m.enrich(ctx, r, enrichOpts{workdirDiag: true, jobStatusLive: true}), nil
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
	if err := m.requireDocker(); err != nil {
		return View{}, err
	}

	name := strings.TrimSpace(req.Name)
	rawURL := strings.TrimSpace(req.URL)
	if name == "" || rawURL == "" {
		return View{}, fmt.Errorf("%w: name and url are required", ErrValidation)
	}
	if err := validateExtraEnv(req.ExtraEnv); err != nil {
		return View{}, err
	}
	if err := validateResourceLimits(req.CPULimit, req.MemoryLimitMB); err != nil {
		return View{}, err
	}
	if err := validateNetworkMode(req.NetworkMode); err != nil {
		return View{}, err
	}
	cache := normalizeCache(req.Cache)
	if err := validateCache(cache); err != nil {
		return View{}, err
	}
	info, err := m.parseProject(rawURL)
	if err != nil {
		return View{}, err
	}

	norm := docker.NormalizeName(name)
	containerName := "gha-runner-" + norm
	// Serialize same-name creates; forget key after so the map does not grow forever.
	unlockName := m.lockRunnerOpt("create:"+containerName, true)
	defer unlockName()

	existing, err := m.Store.List()
	if err != nil {
		return View{}, err
	}
	for _, e := range existing {
		if e.Name == name || e.ContainerName == containerName {
			return View{}, store.ErrConflict
		}
	}

	token, err := m.resolveRegistrationToken(ctx, info.HTMLURL, req.Token)
	if err != nil {
		return View{}, err
	}

	id := uuid.NewString()
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
		Labels:          normalizeLabels(req.Labels),
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
		Cache:           cache,
	}
	rec.WorkdirHostPath = normalizeWorkdirHostPath(rec, req.WorkdirHostPath)
	if err := validateWorkdirHostPath(resolveWorkdirHostPath(rec)); err != nil {
		return View{}, err
	}
	for _, w := range cacheSiblingWarnings(cache) {
		slog.Warn("cache config advisory", "runner", name, "warning", w)
	}

	if !m.createLimiter.Allow() {
		return View{}, fmt.Errorf("%w: too many create/recreate requests", ErrRateLimited)
	}

	// Hold the runner id lock for the rest of create so Delete/Start/Stop/Recreate
	// cannot interleave after Store.Add while startContainer is still running.
	unlockID := m.lockRunner(id)
	defer unlockID()
	if err := m.Store.Add(rec); err != nil {
		return View{}, err
	}

	// Detached lifecycle: client disconnect must not abort after the store row exists.
	dctx, cancel := lifecycleContext()
	defer cancel()
	if err := m.startContainer(dctx, rec, token, info.OrgName()); err != nil {
		return m.rollbackFailedCreate(rec, err)
	}
	return m.Get(dctx, id)
}

// rollbackFailedCreate cleans up after a failed create. Uses a detached Docker
// context so request cancel/timeout cannot leave an orphaned container.
// If GitHub may already have the runner (.runner on the volume or a managed
// container exists), keep the store row and volume. Never soft-success.
func (m *Manager) rollbackFailedCreate(rec store.Runner, createErr error) (View, error) {
	dctx, cancel := docker.DetachedTimeout(detachedOpTimeout)
	defer cancel()

	info, inspErr := m.inspectContainer(dctx, rec.ContainerName)
	if inspErr != nil && !errors.Is(inspErr, errInspectUnavailable) {
		slog.Warn("create rollback: inspect failed; keeping store row",
			"runner", rec.ID, "container", rec.ContainerName, "err", inspErr)
		return View{}, createErr
	}

	managed := inspErr == nil && info.Exists && info.Labels[docker.LabelID] == rec.ID
	hasRunner := false
	if rec.VolumeName != "" {
		if wf, err := m.readAgentWorkFolder(dctx, rec.VolumeName, true); err == nil && strings.TrimSpace(wf) != "" {
			hasRunner = true
		}
	}

	if managed || hasRunner {
		if m.Docker != nil && info.Exists {
			if docker.EnvHasKey(info.Env, "RUNNER_TOKEN") {
				if remErr := m.Docker.RemoveContainerTimeout(dctx, rec.ContainerName, 10); remErr != nil && !docker.IsNotFound(remErr) {
					slog.Warn("create rollback: remove leftover configure container",
						"runner", rec.ID, "err", remErr)
				}
			} else if managed && !info.Running {
				if startErr := m.Docker.Start(dctx, rec.ContainerName); startErr != nil {
					slog.Warn("create rollback: container exists but start failed",
						"runner", rec.ID, "container", rec.ContainerName, "err", startErr)
				}
			}
		}
		slog.Warn("create failed after registration artifact existed; keeping runner for recovery",
			"runner", rec.ID, "container", rec.ContainerName, "has_runner", hasRunner, "managed", managed, "err", createErr)
		return View{}, createErr
	}

	if m.PATConfigured() {
		if derErr := m.GitHub.DeregisterRunner(dctx, rec.URL, rec.Name); derErr != nil {
			slog.Warn("create rollback: github deregister failed", "runner", rec.Name, "err", derErr)
		}
	}
	if m.Docker != nil {
		if remErr := m.Docker.Remove(dctx, rec.ContainerName, rec.VolumeName); remErr != nil && !docker.IsNotFound(remErr) {
			slog.Error("create rollback: remove failed; leaving store row for operator cleanup",
				"runner", rec.ID, "container", rec.ContainerName, "err", remErr)
			return View{}, fmt.Errorf("%w (cleanup failed: %v)", createErr, remErr)
		}
		m.removeOwnedPersistenceVolumes(dctx, rec)
	}
	_ = m.Store.Delete(rec.ID)
	return View{}, createErr
}

// removeOwnedPersistenceVolumes removes auto-named cache volumes and obsolete *-work volumes.
func (m *Manager) removeOwnedPersistenceVolumes(ctx context.Context, rec store.Runner) {
	m.removeLegacyWorkVolume(ctx, rec)
	if cacheVolumeOwned(rec) {
		if name := resolveCacheVolumeName(rec); name != "" {
			if err := m.Docker.RemoveVolume(ctx, name); err != nil && !docker.IsNotFound(err) {
				slog.Warn("remove cache volume", "volume", name, "err", err)
			}
		}
	}
}

func (m *Manager) removeLegacyWorkVolume(ctx context.Context, rec store.Runner) {
	if vol := legacyWorkVolumeName(rec); vol != "" {
		if err := m.Docker.RemoveVolume(ctx, vol); err != nil && !docker.IsNotFound(err) {
			slog.Warn("remove legacy work volume", "volume", vol, "err", err)
		}
	}
}

// startContainer starts the runner container. When token is non-empty it runs a
// configure-only phase (token + no-op CMD, no DEBUG_ONLY) then a tokenless
// long-running container so docker inspect never retains RUNNER_TOKEN and we
// never kill a live GitHub session to scrub it. When token is empty, registration
// files on the volume are reused.
func (m *Manager) startContainer(ctx context.Context, rec store.Runner, token, orgName string) error {
	if token != "" {
		if err := m.configureThenRun(ctx, rec, token, orgName); err != nil {
			return err
		}
		return m.verifyAgentWorkdir(ctx, rec)
	}
	if err := m.startContainerWithoutVerify(ctx, rec, "", orgName, false); err != nil {
		return err
	}
	return m.verifyAgentWorkdir(ctx, rec)
}

// configureThenRun registers via a one-shot container (token, CMD true, no listener),
// then starts the listener without RUNNER_TOKEN. Avoids the old scrubToken stop/start
// race that produced "A session for this runner already exists". DEBUG_ONLY is not
// used: upstream skips config.sh when that flag is set and .runner is missing.
func (m *Manager) configureThenRun(ctx context.Context, rec store.Runner, token, orgName string) error {
	if err := m.startContainerWithoutVerify(ctx, rec, token, orgName, true); err != nil {
		return fmt.Errorf("configure runner: %w", err)
	}
	if err := m.waitForConfigure(ctx, rec); err != nil {
		slog.Warn("configure failed", "runner", rec.Name, "id", rec.ID, "err", err)
		dctx, cancel := docker.DetachedTimeout(detachedOpTimeout)
		_ = m.Docker.RemoveContainerTimeout(dctx, rec.ContainerName, 10)
		cancel()
		return err
	}
	dctx, cancel := docker.DetachedTimeout(detachedOpTimeout)
	defer cancel()
	if err := m.Docker.RemoveContainerTimeout(dctx, rec.ContainerName, 30); err != nil {
		return fmt.Errorf("remove configure container: %w", err)
	}
	if err := m.startContainerWithoutVerify(ctx, rec, "", orgName, false); err != nil {
		return fmt.Errorf("start runner: %w", err)
	}
	if confirmed, err := m.waitForListening(ctx, rec.ContainerName); err != nil {
		slog.Warn("listen wait failed", "runner", rec.Name, "id", rec.ID, "err", err)
		return err
	} else if !confirmed {
		return fmt.Errorf("%w: timed out waiting for runner to listen", ErrValidation)
	}
	return nil
}

func (m *Manager) ensurePersistenceHostDirs(ctx context.Context, rec store.Runner, workdirBind string) error {
	wh := m.workdirHostOrDocker()
	if wh == nil {
		return ErrDockerUnavailable
	}
	if err := wh.EnsureHostDir(ctx, workdirBind); err != nil {
		return fmt.Errorf("ensure workdir host path: %w", err)
	}
	if err := m.installJobHooks(ctx, workdirBind); err != nil {
		return err
	}
	if rec.Cache != nil && rec.Cache.Enabled && cacheType(rec.Cache) == "bind" {
		if hp := strings.TrimSpace(rec.Cache.HostPath); hp != "" {
			if err := wh.EnsureHostDir(ctx, hp); err != nil {
				return fmt.Errorf("ensure cache host path: %w", err)
			}
		}
	}
	return nil
}

func (m *Manager) startContainerWithoutVerify(ctx context.Context, rec store.Runner, token, orgName string, configureOnly bool) error {
	workdirBind := resolveWorkdirHostPath(rec)
	if err := validateWorkdirHostPath(workdirBind); err != nil {
		return err
	}
	if err := m.ensurePersistenceHostDirs(ctx, rec, workdirBind); err != nil {
		return err
	}
	env := m.buildEnv(rec, token, orgName, workdirBind)
	mountSock := m.MountDockerSock
	if rec.MountDockerSock != nil {
		mountSock = *rec.MountDockerSock
	}
	extra := buildExtraMounts(rec, workdirBind)
	restart := "unless-stopped"
	var stopSecs *int
	var cmd []string
	if configureOnly {
		// One-shot configure: must not restart-loop after entrypoint exits.
		restart = "no"
		cmd = []string{configureNoopCmd}
	} else {
		s := StopTimeoutSecs
		stopSecs = &s
	}
	_, err := m.createAndStart(ctx, docker.CreateOpts{
		Name:  rec.ContainerName,
		Image: rec.Image,
		Env:   env,
		Labels: map[string]string{
			docker.LabelManaged: "true",
			docker.LabelID:      rec.ID,
		},
		VolumeName:       rec.VolumeName,
		VolumeTarget:     configFilesDir,
		ExtraMounts:      extra,
		MountDockerSock:  mountSock,
		RestartPolicy:    restart,
		Cmd:              cmd,
		StopTimeout:      stopSecs,
		CPULimit:         rec.CPULimit,
		MemoryLimitMB:    rec.MemoryLimitMB,
		NetworkMode:      rec.NetworkMode,
		AllowRunnerToken: configureOnly,
	})
	if err != nil {
		if docker.IsConflict(err) {
			return fmt.Errorf("%w: container or volume name already exists", store.ErrConflict)
		}
		if errors.Is(err, docker.ErrImageNotFound) {
			return fmt.Errorf("%w: %w", ErrValidation, err)
		}
		if errors.Is(err, docker.ErrImagePull) {
			return fmt.Errorf("%w: %w", ErrImagePull, err)
		}
		return fmt.Errorf("create container: %w", err)
	}
	return nil
}

func (m *Manager) createAndStart(ctx context.Context, opts docker.CreateOpts) (string, error) {
	if m.createAndStartFn != nil {
		return m.createAndStartFn(ctx, opts)
	}
	if m.Docker == nil {
		return "", ErrDockerUnavailable
	}
	return m.Docker.CreateAndStart(ctx, opts)
}

// verifyAgentWorkdir retries reading .runner until workFolder matches the desired host bind.
// Uses a detached deadline so a canceled client/ingress request cannot abort verification
// after the runner container is already running.
func (m *Manager) verifyAgentWorkdir(_ context.Context, rec store.Runner) error {
	desired := resolveWorkdirHostPath(rec)
	m.invalidateAgentWorkFolder(rec.VolumeName)
	timeout := m.verifyTimeout
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	dctx, cancel := docker.DetachedTimeout(timeout)
	defer cancel()
	var lastErr error
	for {
		select {
		case <-dctx.Done():
			if lastErr != nil {
				return fmt.Errorf("%w: agent workFolder not set to %s after start: %v", ErrValidation, desired, lastErr)
			}
			return fmt.Errorf("%w: agent workFolder not set to %s after start: %v", ErrValidation, desired, dctx.Err())
		default:
		}
		wf, err := m.readAgentWorkFolderPreferContainer(dctx, rec, true)
		if err == nil && workdirPathsMatch(wf, desired) {
			return nil
		}
		lastErr = err
		if err == nil {
			lastErr = fmt.Errorf("workFolder=%q want %q", wf, desired)
		}
		select {
		case <-dctx.Done():
			return fmt.Errorf("%w: agent workFolder not set to %s after start: %v", ErrValidation, desired, lastErr)
		case <-time.After(1 * time.Second):
		}
	}
}

func (m *Manager) buildEnv(rec store.Runner, token, orgName, workdir string) []string {
	env := []string{
		"RUNNER_NAME=" + rec.Name,
		"LABELS=" + strings.Join(rec.Labels, ","),
		"RUNNER_SCOPE=" + rec.Scope,
		"CONFIGURED_ACTIONS_RUNNER_FILES_DIR=" + configFilesDir,
		"RUNNER_WORKDIR=" + workdir,
		"DISABLE_AUTO_UPDATE=true",
		"DISABLE_AUTOMATIC_DEREGISTRATION=true",
		"ACTIONS_RUNNER_HOOK_JOB_STARTED=" + jobStartedHookPath(workdir),
		"ACTIONS_RUNNER_HOOK_JOB_COMPLETED=" + jobCompletedHookPath(workdir),
	}
	if cachePath := resolveCacheEffective(rec.Cache); cachePath != "" && cachePath != "." {
		env = append(env, "RUNNER_CACHE="+cachePath)
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

// waitForConfigure waits until the configure-only container wrote .runner (or log
// success markers). Exited without .runner is a hard failure. Soft client cancel
// succeeds only if .runner is already present.
func (m *Manager) waitForConfigure(ctx context.Context, rec store.Runner) error {
	deadline := time.Now().Add(90 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			dctx, cancel := docker.DetachedContext()
			ok := m.agentConfigured(dctx, rec)
			cancel()
			if ok {
				return nil
			}
			return fmt.Errorf("%w: configure interrupted before .runner was written: %v", ErrValidation, ctx.Err())
		default:
		}
		if m.agentConfigured(ctx, rec) {
			return nil
		}
		logs, err := m.dockerTailLogs(ctx, rec.ContainerName, "80")
		if err == nil {
			last = logs
			if err := registrationLogFailure(logs); err != nil {
				return err
			}
			for _, marker := range configureSuccessMarkers {
				if strings.Contains(logs, marker) && m.agentConfigured(ctx, rec) {
					return nil
				}
			}
		}
		info, err := m.inspectContainer(ctx, rec.ContainerName)
		if err == nil && info.Exists && !info.Running && info.Status == "exited" {
			if m.agentConfigured(ctx, rec) {
				return nil
			}
			logs, _ := m.dockerTailLogs(ctx, rec.ContainerName, "80")
			if err := registrationLogFailure(logs); err != nil {
				return err
			}
			return fmt.Errorf("%w: configure container exited without writing .runner: %s", ErrValidation, summarizeLogs(logs))
		}
		time.Sleep(2 * time.Second)
	}
	if m.agentConfigured(ctx, rec) {
		return nil
	}
	if last != "" {
		return fmt.Errorf("%w: timed out waiting for runner configure: %s", ErrValidation, summarizeLogs(last))
	}
	return fmt.Errorf("%w: timed out waiting for runner configure", ErrValidation)
}

func runnerConfigContainerPath() string {
	return configFilesDir + "/" + runnerConfigFile
}

// agentConfigured reports whether .runner exists with a workFolder. Prefers
// CopyFromContainer on the configure/run container to avoid alpine volume helpers.
func (m *Manager) agentConfigured(ctx context.Context, rec store.Runner) bool {
	wf, err := m.readAgentWorkFolderPreferContainer(ctx, rec, true)
	return err == nil && strings.TrimSpace(wf) != ""
}

func (m *Manager) readAgentWorkFolderPreferContainer(ctx context.Context, rec store.Runner, bypassCache bool) (string, error) {
	if rec.ContainerName != "" && m.hasContainerFileReader() {
		data, err := m.readContainerFile(ctx, rec.ContainerName, runnerConfigContainerPath())
		if err == nil {
			wf, perr := parseRunnerWorkFolder(data)
			if perr != nil {
				return "", perr
			}
			if rec.VolumeName != "" {
				m.cacheAgentWorkFolder(rec.VolumeName, wf)
			}
			return wf, nil
		}
		// File/container not found: do not alpine-read the volume (wait/Get storms).
		if isContainerConfigMissing(err) {
			return "", errNoRunnerConfig
		}
		if !errors.Is(err, errContainerReadUnavailable) {
			return "", err
		}
	}
	if rec.VolumeName == "" {
		return "", errNoRunnerConfig
	}
	return m.readAgentWorkFolder(ctx, rec.VolumeName, bypassCache)
}

func (m *Manager) hasContainerFileReader() bool {
	return m.readContainerFileFn != nil || m.Docker != nil
}

func (m *Manager) readContainerFile(ctx context.Context, name, path string) ([]byte, error) {
	if m.readContainerFileFn != nil {
		return m.readContainerFileFn(ctx, name, path)
	}
	if m.Docker == nil {
		return nil, errContainerReadUnavailable
	}
	return m.Docker.ReadContainerFile(ctx, name, path)
}

func isContainerConfigMissing(err error) bool {
	return errors.Is(err, docker.ErrContainerFileNotFound) || docker.IsNotFound(err)
}

// waitForListening returns (confirmed, err). Timeout or cancel while the
// container is still running fails closed (never 201/success).
func (m *Manager) waitForListening(ctx context.Context, containerName string) (bool, error) {
	timeout := m.listenTimeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			dctx, cancel := docker.DetachedContext()
			info, err := m.inspectContainer(dctx, containerName)
			cancel()
			if err == nil && info.Exists && info.Running {
				return false, fmt.Errorf("%w: listen wait interrupted while container still running: %v", ErrValidation, ctx.Err())
			}
			return false, ctx.Err()
		default:
		}
		info, err := m.inspectContainer(ctx, containerName)
		if err == nil && info.Exists && !info.Running && info.Status == "exited" {
			logs, _ := m.dockerTailLogs(ctx, containerName, "80")
			if failErr := registrationLogFailure(logs); failErr != nil {
				return false, failErr
			}
			return false, fmt.Errorf("%w: runner container exited during startup: %s", ErrValidation, summarizeLogs(logs))
		}
		logs, err := m.dockerTailLogs(ctx, containerName, "80")
		if err == nil {
			last = logs
			if failErr := registrationLogFailure(logs); failErr != nil {
				return false, failErr
			}
			for _, marker := range listenSuccessMarkers {
				if strings.Contains(logs, marker) {
					return true, nil
				}
			}
		}
		wait := 2 * time.Second
		if remaining := time.Until(deadline); remaining < wait {
			if remaining <= 0 {
				break
			}
			wait = remaining
		}
		time.Sleep(wait)
	}
	info, err := m.inspectContainer(ctx, containerName)
	if err == nil && info.Exists && info.Running {
		slog.Warn("listen wait timed out without success markers; container still running", "container", containerName, "tail", summarizeLogs(last))
		return false, fmt.Errorf("%w: timed out waiting for runner to listen", ErrValidation)
	}
	if last != "" {
		return false, fmt.Errorf("%w: timed out waiting for runner to listen: %s", ErrValidation, summarizeLogs(last))
	}
	return false, fmt.Errorf("%w: timed out waiting for runner to listen", ErrValidation)
}

func (m *Manager) dockerTailLogs(ctx context.Context, name, tail string) (string, error) {
	if m.Docker == nil {
		return "", ErrDockerUnavailable
	}
	return m.Docker.TailLogs(ctx, name, tail)
}

func registrationLogFailure(logs string) error {
	lower := strings.ToLower(logs)
	for _, marker := range registrationFailureMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return fmt.Errorf("%w: registration failed: %s", ErrValidation, summarizeLogs(logs))
		}
	}
	return nil
}

func summarizeLogs(logs string) string {
	logs = RedactSecrets(strings.TrimSpace(logs))
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

func (m *Manager) Patch(ctx context.Context, id string, req PatchRequest) (View, error) {
	unlock := m.lockRunner(id)
	defer unlock()

	before, err := m.Store.Get(id)
	if err != nil {
		return View{}, err
	}
	rec := before
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
		if err := validateNetworkMode(*req.NetworkMode); err != nil {
			return View{}, err
		}
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
	if req.Cache != nil {
		if !req.Cache.Enabled {
			rec.Cache = nil
		} else {
			rec.Cache = normalizeCache(req.Cache)
		}
	}
	if req.WorkdirHostPath != nil {
		rec.WorkdirHostPath = normalizeWorkdirHostPath(rec, *req.WorkdirHostPath)
	}
	if err := validateCache(rec.Cache); err != nil {
		return View{}, err
	}
	if err := validateResourceLimits(rec.CPULimit, rec.MemoryLimitMB); err != nil {
		return View{}, err
	}
	for _, w := range cacheSiblingWarnings(rec.Cache) {
		slog.Warn("cache config advisory", "runner", rec.Name, "id", id, "warning", w)
	}
	if err := validateWorkdirHostPath(resolveWorkdirHostPath(rec)); err != nil {
		return View{}, err
	}
	if req.Apply {
		view, recErr := m.recreateRec(ctx, rec, RecreateRequest{Token: req.Token}, true)
		if recErr != nil {
			return View{}, recErr
		}
		if err := m.Store.Update(rec); err != nil {
			slog.Error("apply succeeded but store persist failed", "runner", rec.ID, "err", err)
			return view, fmt.Errorf("%w: container applied but failed to persist config: %v", ErrValidation, err)
		}
		m.cleanupStalePersistenceVolumes(ctx, before, rec)
		return m.Get(ctx, id)
	}
	if err := m.Store.Update(rec); err != nil {
		return View{}, err
	}
	return m.Get(ctx, id)
}

// cleanupStalePersistenceVolumes removes cache volumes that the runner no longer
// references after a successful apply. Shared cache volumes are removed only when
// unreferenced by every store runner.
func (m *Manager) cleanupStalePersistenceVolumes(ctx context.Context, before, after store.Runner) {
	if m.Docker == nil {
		return
	}
	prevCache := resolveCacheVolumeName(before)
	nextCache := resolveCacheVolumeName(after)
	if prevCache == "" || prevCache == nextCache {
		return
	}
	runners, err := m.Store.List()
	if err != nil {
		slog.Warn("cleanup: list for cache refcount", "err", err)
		if cacheVolumeOwned(before) {
			if remErr := m.Docker.RemoveVolume(ctx, prevCache); remErr != nil && !docker.IsNotFound(remErr) {
				slog.Warn("cleanup: remove owned cache volume", "volume", prevCache, "err", remErr)
			}
		}
		return
	}
	if cacheVolumeRefs(runners, prevCache) == 0 {
		if err := m.Docker.RemoveVolume(ctx, prevCache); err != nil && !docker.IsNotFound(err) {
			slog.Warn("cleanup: remove stale cache volume", "volume", prevCache, "err", err)
		}
	}
}

func (m *Manager) Recreate(ctx context.Context, id string, req RecreateRequest) (View, error) {
	unlock := m.lockRunner(id)
	defer unlock()
	rec, err := m.Store.Get(id)
	if err != nil {
		return View{}, err
	}
	return m.recreateRec(ctx, rec, req, true)
}

func (m *Manager) RecreateMissing(ctx context.Context, req RecreateMissingRequest) (RecreateMissingResult, error) {
	out := RecreateMissingResult{
		Recreated: []string{},
		Failed:    []RecreateMissingFailure{},
	}
	runners, err := m.Store.List()
	if err != nil {
		return out, err
	}
	if len(runners) == 0 {
		return out, nil
	}
	if err := m.requireDocker(); err != nil {
		return out, err
	}
	views, err := m.List(ctx)
	if err != nil {
		return out, err
	}
	var missing []store.Runner
	for _, v := range views {
		if v.Status == "missing" {
			missing = append(missing, v.Runner)
		}
	}
	if len(missing) == 0 {
		return out, nil
	}
	if !m.createLimiter.Allow() {
		return out, fmt.Errorf("%w: too many create/recreate requests", ErrRateLimited)
	}
	for _, rec := range missing {
		unlock := m.lockRunner(rec.ID)
		view, recErr := m.recreateRec(ctx, rec, RecreateRequest{Token: req.Token}, false)
		unlock()
		if recErr != nil {
			out.Failed = append(out.Failed, RecreateMissingFailure{
				ID:    rec.ID,
				Name:  rec.Name,
				Error: RedactSecrets(recErr.Error()),
			})
			continue
		}
		out.Recreated = append(out.Recreated, view.ID)
	}
	return out, nil
}

func (m *Manager) recreateRec(ctx context.Context, rec store.Runner, req RecreateRequest, consumeQuota bool) (View, error) {
	if err := m.requireDocker(); err != nil {
		return View{}, err
	}
	if err := m.errIfBusy(ctx, rec); err != nil {
		return View{}, err
	}
	info, err := m.parseProject(rec.URL)
	if err != nil {
		return View{}, err
	}
	desiredWork := resolveWorkdirHostPath(rec)
	if err := validateWorkdirHostPath(desiredWork); err != nil {
		return View{}, err
	}

	dctx, cancel := lifecycleContext()
	defer cancel()

	containerInfo, err := m.inspectContainer(dctx, rec.ContainerName)
	if err != nil {
		slog.Warn("recreate: inspect container failed", "runner", rec.Name, "id", rec.ID, "err", err)
		return View{}, wrapInspectErr(err)
	}
	volExists, volErr := m.Docker.VolumeExists(dctx, rec.VolumeName)
	if volErr != nil {
		return View{}, fmt.Errorf("inspect registration volume: %w", volErr)
	}

	var agentWF string
	var agentErr error
	if volExists {
		agentWF, agentErr = m.readAgentWorkFolder(dctx, rec.VolumeName, true)
	}
	plan, planErr := planWorkdirReconfigure(volExists, agentWF, agentErr, desiredWork)
	if planErr != nil {
		return View{}, planErr
	}
	if plan.Needs {
		slog.Info("recreate: workdir reconfigure required", "reason", plan.Reason, "desired", desiredWork, "agent", agentWF)
	}

	token, resolveErr := m.resolveRecreateToken(dctx, rec.URL, req.Token, plan.Needs)
	if resolveErr != nil {
		reason := plan.Reason
		if reason == "" {
			reason = "reconfigure required"
		}
		slog.Warn("recreate: registration token required", "runner", rec.Name, "id", rec.ID, "reason", reason)
		return View{}, fmt.Errorf("%w: %s — provide a registration token or configure GITHUB_PAT (agent workFolder is set only at configure time)", ErrValidation, reason)
	}

	if consumeQuota && !m.createLimiter.Allow() {
		return View{}, fmt.Errorf("%w: too many create/recreate requests", ErrRateLimited)
	}

	if containerInfo.Exists {
		if err := m.Docker.RemoveContainerTimeout(dctx, rec.ContainerName, StopTimeoutSecs); err != nil && !docker.IsNotFound(err) {
			return View{}, fmt.Errorf("remove container: %w", err)
		}
	}
	cleared := false
	if plan.Needs && volExists {
		if err := m.backupRunnerConfig(dctx, rec.VolumeName); err != nil {
			return View{}, fmt.Errorf("backup .runner: %w", err)
		}
		if err := m.clearRunnerConfigForReconfigure(dctx, rec.VolumeName); err != nil {
			return View{}, fmt.Errorf("clear .runner for workdir reconfigure: %w", err)
		}
		cleared = true
	}
	m.removeLegacyWorkVolume(dctx, rec)
	if err := m.startContainer(dctx, rec, token, info.OrgName()); err != nil {
		slog.Warn("recreate: start failed", "runner", rec.Name, "id", rec.ID, "err", err)
		if cleared {
			if restErr := m.restoreRunnerConfig(dctx, rec.VolumeName); restErr != nil {
				slog.Error("recreate: restore .runner after start failure", "runner", rec.ID, "err", restErr)
			}
		}
		return View{}, err
	}
	return m.Get(dctx, rec.ID)
}

func (m *Manager) backupRunnerConfig(ctx context.Context, volumeName string) error {
	if volumeName == "" {
		return nil
	}
	wh := m.workdirHostOrDocker()
	if wh == nil {
		return ErrDockerUnavailable
	}
	data, err := wh.ReadVolumeFile(ctx, volumeName, runnerConfigFile)
	if err != nil {
		if errors.Is(err, docker.ErrVolumeFileNotFound) || errors.Is(err, errNoRunnerConfig) {
			return nil
		}
		return err
	}
	return wh.WriteVolumeFile(ctx, volumeName, runnerConfigBackupFile, data)
}

func (m *Manager) restoreRunnerConfig(ctx context.Context, volumeName string) error {
	if volumeName == "" {
		return nil
	}
	wh := m.workdirHostOrDocker()
	if wh == nil {
		return ErrDockerUnavailable
	}
	data, err := wh.ReadVolumeFile(ctx, volumeName, runnerConfigBackupFile)
	if err != nil {
		return err
	}
	if err := wh.WriteVolumeFile(ctx, volumeName, runnerConfigFile, data); err != nil {
		return err
	}
	m.invalidateAgentWorkFolder(volumeName)
	return nil
}

// resolveRecreateToken returns a registration token only when reconfigure is needed.
// Otherwise returns empty so recreate reuses volume credentials without a configure phase.
func (m *Manager) resolveRecreateToken(ctx context.Context, projectURL, reqToken string, needsReconfigure bool) (string, error) {
	if !needsReconfigure {
		return "", nil
	}
	return m.resolveRegistrationToken(ctx, projectURL, reqToken)
}

func (m *Manager) applyLifecycle(ctx context.Context, id string, op func(context.Context, string) error) (View, error) {
	if err := m.requireDocker(); err != nil {
		return View{}, err
	}
	r, err := m.Store.Get(id)
	if err != nil {
		return View{}, err
	}
	if err := op(ctx, r.ContainerName); err != nil {
		return View{}, mapLifecycleDockerErr(err)
	}
	return m.Get(ctx, id)
}

func (m *Manager) Start(ctx context.Context, id string) (View, error) {
	unlock := m.lockRunner(id)
	defer unlock()
	if err := m.requireDocker(); err != nil {
		return View{}, err
	}
	r, err := m.Store.Get(id)
	if err != nil {
		return View{}, err
	}
	if err := m.ensureHooksThenStart(ctx, r); err != nil {
		return View{}, mapLifecycleDockerErr(err)
	}
	return m.Get(ctx, id)
}

func (m *Manager) Stop(ctx context.Context, id string) (View, error) {
	unlock := m.lockRunner(id)
	defer unlock()
	dctx, cancel := docker.DetachedTimeout(detachedLifecycleTimeout)
	defer cancel()
	return m.applyLifecycle(dctx, id, func(ctx context.Context, name string) error {
		return m.Docker.Stop(ctx, name)
	})
}

func (m *Manager) Restart(ctx context.Context, id string) (View, error) {
	unlock := m.lockRunner(id)
	defer unlock()
	if err := m.requireDocker(); err != nil {
		return View{}, err
	}
	r, err := m.Store.Get(id)
	if err != nil {
		return View{}, err
	}
	dctx, cancel := docker.DetachedTimeout(detachedLifecycleTimeout)
	defer cancel()
	if err := m.ensureHooksThenRestart(dctx, r); err != nil {
		return View{}, mapLifecycleDockerErr(err)
	}
	return m.Get(dctx, id)
}

func (m *Manager) Delete(ctx context.Context, id string) error {
	unlock := m.lockRunnerOpt(id, true)
	defer unlock()
	r, err := m.Store.Get(id)
	if err != nil {
		return err
	}
	if err := m.errIfBusy(ctx, r); err != nil {
		return err
	}
	if err := m.requireDocker(); err != nil {
		return err
	}
	dctx, cancel := docker.DetachedTimeout(detachedLifecycleTimeout)
	defer cancel()
	if m.PATConfigured() {
		if err := m.GitHub.DeregisterRunner(dctx, r.URL, r.Name); err != nil {
			slog.Warn("github deregister failed; continuing local delete", "runner", r.Name, "err", err)
		}
	}
	if err := m.Docker.RemoveContainerTimeout(dctx, r.ContainerName, StopTimeoutSecs); err != nil && !docker.IsNotFound(err) {
		return err
	}
	if r.VolumeName != "" {
		if err := m.Docker.RemoveVolume(dctx, r.VolumeName); err != nil && !docker.IsNotFound(err) {
			return fmt.Errorf("remove registration volume: %w", err)
		}
	}
	m.removeLegacyWorkVolume(dctx, r)
	m.invalidateAgentWorkFolder(r.VolumeName)
	if cacheVol := resolveCacheVolumeName(r); cacheVol != "" {
		others, listErr := m.Store.List()
		removeCache := false
		if listErr != nil {
			slog.Warn("delete: list for cache refcount", "err", listErr)
			removeCache = cacheVolumeOwned(r)
		} else {
			removeCache = cacheVolumeRefs(others, cacheVol) <= 1
		}
		if removeCache {
			if err := m.Docker.RemoveVolume(dctx, cacheVol); err != nil && !docker.IsNotFound(err) {
				slog.Warn("delete: remove cache volume", "volume", cacheVol, "err", err)
			}
		}
	}
	return m.Store.Delete(id)
}

func (m *Manager) Logs(ctx context.Context, id string, follow bool, tail string) (io.ReadCloser, error) {
	r, err := m.Store.Get(id)
	if err != nil {
		return nil, err
	}
	if err := m.requireDocker(); err != nil {
		return nil, err
	}
	rc, err := m.Docker.Logs(ctx, r.ContainerName, follow, tail)
	if err != nil {
		return nil, mapLifecycleDockerErr(err)
	}
	return rc, nil
}

// Reconcile syncs Docker managed containers with the store and records orphans.
// Missing containers are not written to the store — List/Get report status=missing
// via live Docker inspect. Orphans are managed-labeled containers with no store row.
func (m *Manager) Reconcile(ctx context.Context) {
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	_ = m.ensureDocker()
	if m.Docker == nil {
		return
	}
	if err := m.Docker.RemoveExitedHelpers(ctx); err != nil {
		slog.Debug("reconcile: remove exited helpers", "err", err)
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
	for k, v := range env {
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
		if strings.ContainsAny(v, "\n\r\x00") {
			return fmt.Errorf("%w: extra_env %q contains invalid characters", ErrValidation, k)
		}
	}
	return nil
}

func validateNetworkMode(mode string) error {
	mode = strings.TrimSpace(mode)
	switch mode {
	case "", "bridge", "host", "none":
		return nil
	}
	if strings.HasPrefix(mode, "container:") {
		id := strings.TrimSpace(strings.TrimPrefix(mode, "container:"))
		if id == "" || strings.ContainsAny(id, " \n\r\t") {
			return fmt.Errorf("%w: invalid network_mode", ErrValidation)
		}
		return nil
	}
	return fmt.Errorf("%w: network_mode must be empty, bridge, host, none, or container:<id>", ErrValidation)
}

func validateResourceLimits(cpu float64, memMB int64) error {
	if cpu < 0 || math.IsNaN(cpu) || math.IsInf(cpu, 0) {
		return fmt.Errorf("%w: cpu_limit must be >= 0", ErrValidation)
	}
	if cpu > maxCPULimit {
		return fmt.Errorf("%w: cpu_limit exceeds %v", ErrValidation, maxCPULimit)
	}
	if memMB < 0 {
		return fmt.Errorf("%w: memory_limit_mb must be >= 0", ErrValidation)
	}
	if memMB > maxMemoryLimitMB {
		return fmt.Errorf("%w: memory_limit_mb exceeds %d", ErrValidation, maxMemoryLimitMB)
	}
	return nil
}

func wrapInspectErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errInspectUnavailable) {
		return ErrDockerUnavailable
	}
	if docker.IsContextError(err) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrDockerUnavailable, err)
}

func mapLifecycleDockerErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrValidation) || errors.Is(err, ErrDockerUnavailable) {
		return err
	}
	if docker.IsNotFound(err) {
		return fmt.Errorf("%w: container is missing; recreate it", ErrValidation)
	}
	if docker.IsContextError(err) {
		return err
	}
	return err
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
