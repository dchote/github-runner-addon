package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"golang.org/x/sync/semaphore"
	"golang.org/x/sync/singleflight"
)

const (
	LabelManaged = "com.github-runner-addon.managed"
	LabelID      = "com.github-runner-addon.id"

	// opTimeout is used for follow-up Docker calls when the caller's context is
	// already canceled (client disconnect / request timeout) but the daemon may
	// have completed the work.
	opTimeout = 45 * time.Second

	// Cap concurrent one-shot alpine helpers so lifecycle storms cannot starve List.
	maxConcurrentHelpers = 4

	inspectCacheTTL = 2 * time.Second
)

// Client wraps the Docker Engine API.
type Client struct {
	cli *client.Client

	helperSem *semaphore.Weighted

	inspectSF    singleflight.Group
	inspectCache sync.Map // name -> inspectCacheEntry
	inspectGens  sync.Map // name -> *atomic.Uint64 (bumped on invalidate)
}

type inspectCacheEntry struct {
	info    InspectInfo
	expires time.Time
	gen     uint64
}

func New() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Client{
		cli:       cli,
		helperSem: semaphore.NewWeighted(maxConcurrentHelpers),
	}, nil
}

func (c *Client) Close() error {
	if c.cli == nil {
		return nil
	}
	return c.cli.Close()
}

func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err := c.cli.Ping(ctx)
	return err
}

type CreateOpts struct {
	Name            string
	Image           string
	Env             []string
	Labels          map[string]string
	VolumeName      string
	VolumeTarget    string
	ExtraMounts     []mount.Mount // additional volume/bind mounts (cache, workdir)
	MountDockerSock bool
	RestartPolicy   string
	StopTimeout     *int    // seconds; set on container Config (honored by docker stop)
	CPULimit        float64 // CPUs; 0 = unlimited
	MemoryLimitMB   int64   // 0 = unlimited
	NetworkMode     string
}

func (c *Client) EnsureImage(ctx context.Context, image string) error {
	_, _, err := c.cli.ImageInspectWithRaw(ctx, image)
	if err == nil {
		return nil
	}
	reader, err := c.cli.ImagePull(ctx, image, imagetypes.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image: %w", err)
	}
	defer reader.Close()
	_, _ = io.Copy(io.Discard, reader)
	return nil
}

func (c *Client) EnsureVolume(ctx context.Context, name string) error {
	_, err := c.cli.VolumeInspect(ctx, name)
	if err == nil {
		return nil
	}
	_, err = c.cli.VolumeCreate(ctx, volume.CreateOptions{Name: name})
	return err
}

func (c *Client) RemoveVolume(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}
	return c.cli.VolumeRemove(ctx, name, true)
}

// VolumeExists reports whether a named volume is present.
func (c *Client) VolumeExists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	_, err := c.cli.VolumeInspect(ctx, name)
	if err == nil {
		return true, nil
	}
	if client.IsErrNotFound(err) {
		return false, nil
	}
	return false, err
}

// EnvHasKey reports whether inspect env contains KEY= (any value).
func EnvHasKey(env []string, key string) bool {
	prefix := key + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

func (c *Client) CreateAndStart(ctx context.Context, opts CreateOpts) (string, error) {
	if err := c.EnsureImage(ctx, opts.Image); err != nil {
		return "", err
	}
	if opts.VolumeName != "" {
		if err := c.EnsureVolume(ctx, opts.VolumeName); err != nil {
			return "", fmt.Errorf("volume: %w", err)
		}
	}
	for _, mnt := range opts.ExtraMounts {
		if mnt.Type == mount.TypeVolume && mnt.Source != "" {
			if err := c.EnsureVolume(ctx, mnt.Source); err != nil {
				return "", fmt.Errorf("volume %s: %w", mnt.Source, err)
			}
		}
	}

	mounts := []mount.Mount{}
	if opts.VolumeName != "" && opts.VolumeTarget != "" {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: opts.VolumeName,
			Target: opts.VolumeTarget,
		})
	}
	mounts = append(mounts, opts.ExtraMounts...)
	if opts.MountDockerSock {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: "/var/run/docker.sock",
			Target: "/var/run/docker.sock",
		})
	}

	restart := container.RestartPolicy{Name: container.RestartPolicyUnlessStopped}
	if opts.RestartPolicy != "" {
		restart.Name = container.RestartPolicyMode(opts.RestartPolicy)
	}

	hostCfg := &container.HostConfig{
		Mounts:        mounts,
		RestartPolicy: restart,
	}
	if opts.CPULimit > 0 {
		hostCfg.NanoCPUs = int64(opts.CPULimit * 1e9)
	}
	if opts.MemoryLimitMB > 0 {
		hostCfg.Memory = opts.MemoryLimitMB * 1024 * 1024
	}
	if opts.NetworkMode != "" {
		hostCfg.NetworkMode = container.NetworkMode(opts.NetworkMode)
	}

	cfg := &container.Config{
		Image:  opts.Image,
		Env:    opts.Env,
		Labels: opts.Labels,
	}
	if opts.StopTimeout != nil {
		cfg.StopTimeout = opts.StopTimeout
	}

	resp, err := c.cli.ContainerCreate(ctx, cfg, hostCfg, (*network.NetworkingConfig)(nil), nil, opts.Name)
	if err != nil {
		id, adoptErr := c.adoptExisting(opts, err)
		if adoptErr == nil {
			return id, nil
		}
		return "", err
	}
	if err := c.startByID(ctx, resp.ID); err != nil {
		// Context may have been canceled after create succeeded; finish start
		// with a detached deadline before giving up and removing.
		if IsContextError(err) {
			if startErr := c.startByIDDetached(resp.ID); startErr == nil {
				slog.Warn("container start recovered after context cancel", "name", opts.Name, "id", resp.ID)
				c.InvalidateInspect(opts.Name)
				return resp.ID, nil
			}
		}
		_ = c.removeByIDDetached(resp.ID)
		return "", err
	}
	c.InvalidateInspect(opts.Name)
	return resp.ID, nil
}

// adoptExisting recovers when ContainerCreate fails with a name conflict or a
// canceled/deadline context: the daemon may already have created the container.
func (c *Client) adoptExisting(opts CreateOpts, createErr error) (string, error) {
	if !IsConflict(createErr) && !IsContextError(createErr) {
		return "", createErr
	}
	dctx, cancel := DetachedContext()
	defer cancel()

	info, err := c.InspectByName(dctx, opts.Name)
	if err != nil {
		return "", err
	}
	if !info.Exists {
		return "", createErr
	}
	if !labelsMatch(info.Labels, opts.Labels) {
		return "", createErr
	}
	if !info.Running {
		if err := c.cli.ContainerStart(dctx, info.ID, container.StartOptions{}); err != nil {
			return "", fmt.Errorf("start adopted container: %w", err)
		}
	}
	c.InvalidateInspect(opts.Name)
	slog.Warn("adopted existing container after create race",
		"name", opts.Name,
		"id", info.ID,
		"create_err", createErr,
	)
	return info.ID, nil
}

func labelsMatch(have, want map[string]string) bool {
	if want == nil {
		return have[LabelManaged] == "true"
	}
	if id := want[LabelID]; id != "" {
		return have[LabelManaged] == "true" && have[LabelID] == id
	}
	return have[LabelManaged] == "true"
}

func (c *Client) startByID(ctx context.Context, id string) error {
	return c.cli.ContainerStart(ctx, id, container.StartOptions{})
}

func (c *Client) startByIDDetached(id string) error {
	dctx, cancel := DetachedContext()
	defer cancel()
	return c.cli.ContainerStart(dctx, id, container.StartOptions{})
}

func (c *Client) removeByIDDetached(id string) error {
	dctx, cancel := DetachedContext()
	defer cancel()
	return c.cli.ContainerRemove(dctx, id, container.RemoveOptions{Force: true})
}

type InspectInfo struct {
	ID      string
	Name    string
	Status  string // Docker raw status
	Running bool
	Exists  bool
	Env     []string
	Labels  map[string]string
}

func (c *Client) InspectByName(ctx context.Context, name string) (InspectInfo, error) {
	if c == nil {
		return InspectInfo{Name: name}, fmt.Errorf("docker client nil")
	}
	if v, ok := c.getInspectCache(name); ok {
		return v, nil
	}
	// Coalesce concurrent inspects per (name, generation). Invalidate bumps
	// generation so in-flight puts cannot re-poison the cache, and new callers
	// start a fresh flight instead of sharing a stale one.
	gen := c.inspectGen(name)
	key := fmt.Sprintf("%s#%d", name, gen)
	v, err, _ := c.inspectSF.Do(key, func() (any, error) {
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		info, err := c.inspectByNameUncached(dctx, name)
		if err == nil {
			c.putInspectCache(name, info, gen)
		}
		return info, err
	})
	if err != nil {
		return InspectInfo{Name: name}, err
	}
	info, _ := v.(InspectInfo)
	return info, nil
}

func (c *Client) inspectByNameUncached(ctx context.Context, name string) (InspectInfo, error) {
	info := InspectInfo{Name: name}
	cj, err := c.cli.ContainerInspect(ctx, name)
	if err != nil {
		if client.IsErrNotFound(err) {
			return info, nil
		}
		return info, err
	}
	info.Exists = true
	info.ID = cj.ID
	info.Status = cj.State.Status
	info.Running = cj.State.Running
	if cj.Config != nil {
		info.Env = append([]string{}, cj.Config.Env...)
		if len(cj.Config.Labels) > 0 {
			info.Labels = make(map[string]string, len(cj.Config.Labels))
			for k, v := range cj.Config.Labels {
				info.Labels[k] = v
			}
		}
	}
	return info, nil
}

func (c *Client) inspectGen(name string) uint64 {
	v, _ := c.inspectGens.LoadOrStore(name, new(atomic.Uint64))
	return v.(*atomic.Uint64).Load()
}

func (c *Client) bumpInspectGen(name string) uint64 {
	v, _ := c.inspectGens.LoadOrStore(name, new(atomic.Uint64))
	return v.(*atomic.Uint64).Add(1)
}

func (c *Client) getInspectCache(name string) (InspectInfo, bool) {
	v, ok := c.inspectCache.Load(name)
	if !ok {
		return InspectInfo{}, false
	}
	ent := v.(inspectCacheEntry)
	if time.Now().After(ent.expires) || ent.gen != c.inspectGen(name) {
		c.inspectCache.Delete(name)
		return InspectInfo{}, false
	}
	return ent.info, true
}

func (c *Client) putInspectCache(name string, info InspectInfo, gen uint64) {
	if c.inspectGen(name) != gen {
		return // invalidated while inspect was in flight
	}
	c.inspectCache.Store(name, inspectCacheEntry{
		info:    info,
		expires: time.Now().Add(inspectCacheTTL),
		gen:     gen,
	})
}

// InvalidateInspect drops cached inspect data after lifecycle mutations and
// bumps the per-name generation so in-flight singleflight puts are discarded.
func (c *Client) InvalidateInspect(name string) {
	if c == nil || name == "" {
		return
	}
	c.inspectCache.Delete(name)
	c.bumpInspectGen(name)
}

// ManagedContainer is a Docker container labeled as managed by this addon.
type ManagedContainer struct {
	Name     string
	ID       string
	RunnerID string
	Status   string
	Running  bool
}

// ListManaged returns containers with the managed label.
func (c *Client) ListManaged(ctx context.Context) ([]ManagedContainer, error) {
	args := filters.NewArgs()
	args.Add("label", LabelManaged+"=true")
	list, err := c.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: args})
	if err != nil {
		return nil, err
	}
	out := make([]ManagedContainer, 0, len(list))
	for _, item := range list {
		name := ""
		if len(item.Names) > 0 {
			name = strings.TrimPrefix(item.Names[0], "/")
		}
		out = append(out, ManagedContainer{
			Name:     name,
			ID:       item.ID,
			RunnerID: item.Labels[LabelID],
			Status:   item.State,
			Running:  item.State == "running",
		})
	}
	return out, nil
}

func (c *Client) Start(ctx context.Context, name string) error {
	err := c.cli.ContainerStart(ctx, name, container.StartOptions{})
	c.InvalidateInspect(name)
	return err
}

const defaultStopTimeoutSecs = 30

// Stop stops the container. Uses Config.StopTimeout when set, otherwise 30s.
func (c *Client) Stop(ctx context.Context, name string) error {
	return c.StopTimeout(ctx, name, c.lookupStopTimeout(ctx, name))
}

// StopTimeout stops the container with an explicit grace period (seconds).
// Values <= 0 fall back to Config.StopTimeout or 30s.
func (c *Client) StopTimeout(ctx context.Context, name string, secs int) error {
	if secs <= 0 {
		secs = c.lookupStopTimeout(ctx, name)
	}
	err := c.cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &secs})
	c.InvalidateInspect(name)
	return err
}

// Restart restarts the container using Config.StopTimeout when set, otherwise 30s.
func (c *Client) Restart(ctx context.Context, name string) error {
	secs := c.lookupStopTimeout(ctx, name)
	err := c.cli.ContainerRestart(ctx, name, container.StopOptions{Timeout: &secs})
	c.InvalidateInspect(name)
	return err
}

func (c *Client) lookupStopTimeout(ctx context.Context, name string) int {
	cj, err := c.cli.ContainerInspect(ctx, name)
	if err == nil && cj.Config != nil && cj.Config.StopTimeout != nil && *cj.Config.StopTimeout > 0 {
		return *cj.Config.StopTimeout
	}
	return defaultStopTimeoutSecs
}

// RemoveContainer removes the container but leaves the volume intact.
func (c *Client) RemoveContainer(ctx context.Context, name string) error {
	return c.RemoveContainerTimeout(ctx, name, 0)
}

// RemoveContainerTimeout stops (with secs grace when > 0) then removes the container.
// secs <= 0 uses Config.StopTimeout or the 30s default.
func (c *Client) RemoveContainerTimeout(ctx context.Context, name string, secs int) error {
	inspect, err := c.InspectByName(ctx, name)
	if err != nil {
		return err
	}
	if !inspect.Exists {
		return nil
	}
	if inspect.Running {
		_ = c.StopTimeout(ctx, name, secs)
	}
	if err := c.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true}); err != nil {
		if !client.IsErrNotFound(err) {
			return err
		}
	}
	c.InvalidateInspect(name)
	return nil
}

func (c *Client) Remove(ctx context.Context, name string, removeVolume string) error {
	if err := c.RemoveContainer(ctx, name); err != nil {
		return err
	}
	if removeVolume != "" {
		_ = c.RemoveVolume(ctx, removeVolume)
	}
	return nil
}

// IsNotFound reports whether err is a Docker "not found" error.
func IsNotFound(err error) bool {
	return err != nil && client.IsErrNotFound(err)
}

// IsConflict reports whether err indicates a name conflict.
func IsConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already in use") ||
		strings.Contains(msg, "conflict") ||
		strings.Contains(msg, "is already taken")
}

// IsContextError reports whether err is (or wraps) a canceled/deadline context.
func IsContextError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Some HTTP/Docker transport errors only embed the message.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "context deadline exceeded")
}

// DetachedTimeout returns a background context with the given timeout for Docker
// work that must outlive a canceled client/ingress request.
func DetachedTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		d = opTimeout
	}
	return context.WithTimeout(context.Background(), d)
}

// DetachedContext returns a background context with opTimeout for short cleanup
// when the caller's context may already be canceled.
func DetachedContext() (context.Context, context.CancelFunc) {
	return DetachedTimeout(opTimeout)
}

// Logs returns a demultiplexed stdout+stderr stream (no Docker mux headers).
func (c *Client) Logs(ctx context.Context, name string, follow bool, tail string) (io.ReadCloser, error) {
	if tail == "" {
		tail = "200"
	}
	reader, err := c.cli.ContainerLogs(ctx, name, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       tail,
		Timestamps: false,
	})
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	go func() {
		defer reader.Close()
		_, copyErr := stdcopy.StdCopy(pw, pw, reader)
		if copyErr != nil {
			_ = pw.CloseWithError(copyErr)
			return
		}
		_ = pw.Close()
	}()
	return pr, nil
}

// TailLogs returns the last N lines of demultiplexed logs as a string.
func (c *Client) TailLogs(ctx context.Context, name string, tail string) (string, error) {
	rc, err := c.Logs(ctx, name, false, tail)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func NormalizeName(name string) string {
	n := strings.ToLower(name)
	n = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, n)
	n = strings.Trim(n, "-_")
	if n == "" {
		n = "runner"
	}
	return n
}
