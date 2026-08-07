package docker

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/docker/docker/api/types/mount"
)

// Preferred bind roots for EnsureHostDir (narrowest matching prefix wins).
// Avoid binding "/" unless the path is outside these roots.
var ensureHostDirRoots = []string{
	"/srv",
	"/mnt",
	"/data",
	"/home",
	"/opt",
	"/var",
}

// EnsureHostDir creates dir on the Docker host (mkdir -p) so bind mounts succeed.
// Modern Docker rejects bind sources that do not exist. The addon cannot mkdir on
// the host directly, so this uses a one-shot helper that bind-mounts the narrowest
// known root containing the path (falling back to "/").
func (c *Client) EnsureHostDir(ctx context.Context, hostPath string) error {
	hostPath, err := sanitizeHostPath(hostPath)
	if err != nil {
		return err
	}
	root, rel := hostDirBindRoot(hostPath)
	containerPath := path.Join("/host", rel)
	if rel == "" || rel == "." {
		containerPath = "/host"
	}

	out, code, err := c.runHelper(ctx, []mount.Mount{{
		Type:   mount.TypeBind,
		Source: root,
		Target: "/host",
	}}, []string{"sh", "-c", fmt.Sprintf("mkdir -p %q && test -d %q", containerPath, containerPath)})
	if err != nil {
		return fmt.Errorf("ensure host dir %s: %w", hostPath, err)
	}
	if code != 0 {
		return fmt.Errorf("ensure host dir %s: exit %d: %s", hostPath, code, strings.TrimSpace(out))
	}
	return nil
}

func sanitizeHostPath(hostPath string) (string, error) {
	raw := strings.TrimSpace(hostPath)
	if raw == "" || strings.Contains(raw, "\x00") || strings.Contains(raw, "..") {
		return "", fmt.Errorf("invalid host path %q", hostPath)
	}
	clean := path.Clean(raw)
	if clean == "/" || !strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid host path %q", hostPath)
	}
	return clean, nil
}

// hostDirBindRoot returns the host directory to bind and the path relative to it
// for mkdir inside the helper (joined under /host).
func hostDirBindRoot(hostPath string) (root, rel string) {
	hostPath = path.Clean(hostPath)
	for _, candidate := range ensureHostDirRoots {
		if hostPath == candidate {
			return candidate, "."
		}
		prefix := candidate + "/"
		if strings.HasPrefix(hostPath, prefix) {
			return candidate, strings.TrimPrefix(hostPath, prefix)
		}
	}
	// Last resort: bind "/" and mkdir the absolute path under /host.
	return "/", strings.TrimPrefix(hostPath, "/")
}
