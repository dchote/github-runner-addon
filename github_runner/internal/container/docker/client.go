package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	imagetypes "github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	LabelManaged = "com.github-runner-addon.managed"
	LabelID      = "com.github-runner-addon.id"
)

// Client wraps the Docker Engine API.
type Client struct {
	cli *client.Client
}

func New() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &Client{cli: cli}, nil
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
	MountDockerSock bool
	RestartPolicy   string
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

	mounts := []mount.Mount{}
	if opts.VolumeName != "" && opts.VolumeTarget != "" {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeVolume,
			Source: opts.VolumeName,
			Target: opts.VolumeTarget,
		})
	}
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

	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image:  opts.Image,
		Env:    opts.Env,
		Labels: opts.Labels,
	}, hostCfg, (*network.NetworkingConfig)(nil), nil, opts.Name)
	if err != nil {
		return "", err
	}
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		_ = c.cli.ContainerRemove(ctx, resp.ID, container.RemoveOptions{Force: true})
		return "", err
	}
	return resp.ID, nil
}

type InspectInfo struct {
	ID      string
	Name    string
	Status  string // Docker raw status
	Running bool
	Exists  bool
	Env     []string
}

func (c *Client) InspectByName(ctx context.Context, name string) (InspectInfo, error) {
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
	}
	return info, nil
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
	return c.cli.ContainerStart(ctx, name, container.StartOptions{})
}

func (c *Client) Stop(ctx context.Context, name string) error {
	timeout := 30
	return c.cli.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout})
}

func (c *Client) Restart(ctx context.Context, name string) error {
	timeout := 30
	return c.cli.ContainerRestart(ctx, name, container.StopOptions{Timeout: &timeout})
}

// RemoveContainer removes the container but leaves the volume intact.
func (c *Client) RemoveContainer(ctx context.Context, name string) error {
	inspect, err := c.InspectByName(ctx, name)
	if err != nil {
		return err
	}
	if !inspect.Exists {
		return nil
	}
	if inspect.Running {
		_ = c.Stop(ctx, name)
	}
	if err := c.cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true}); err != nil {
		if !client.IsErrNotFound(err) {
			return err
		}
	}
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
