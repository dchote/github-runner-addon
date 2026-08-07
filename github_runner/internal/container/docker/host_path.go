package docker

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
)

// EnsureHostDir creates dir on the Docker host (mkdir -p) so bind mounts succeed.
// Modern Docker rejects bind sources that do not exist; the addon container cannot
// mkdir on the host filesystem directly, so this uses a one-shot helper with / mounted.
func (c *Client) EnsureHostDir(ctx context.Context, hostPath string) error {
	raw := strings.TrimSpace(hostPath)
	if raw == "" || strings.Contains(raw, "\x00") || strings.Contains(raw, "..") {
		return fmt.Errorf("invalid host path %q", hostPath)
	}
	hostPath = path.Clean(raw)
	if hostPath == "/" || !strings.HasPrefix(hostPath, "/") {
		return fmt.Errorf("invalid host path %q", hostPath)
	}

	out, code, err := c.runHostHelper(ctx, []string{"mkdir", "-p", "/host" + hostPath})
	if err != nil {
		return fmt.Errorf("ensure host dir %s: %w", hostPath, err)
	}
	if code != 0 {
		return fmt.Errorf("ensure host dir %s: exit %d: %s", hostPath, code, strings.TrimSpace(out))
	}
	return nil
}

func (c *Client) runHostHelper(ctx context.Context, cmd []string) (string, int, error) {
	if err := c.EnsureImage(ctx, volumeHelperImage); err != nil {
		return "", -1, fmt.Errorf("host helper image: %w", err)
	}
	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image: volumeHelperImage,
		Cmd:   cmd,
	}, &container.HostConfig{
		Mounts: []mount.Mount{{
			Type:   mount.TypeBind,
			Source: "/",
			Target: "/host",
		}},
	}, nil, nil, "")
	if err != nil {
		return "", -1, err
	}
	return c.waitHelperLogs(ctx, resp.ID)
}
