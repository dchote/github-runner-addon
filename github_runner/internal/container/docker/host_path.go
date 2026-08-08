package docker

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/docker/docker/api/types/mount"
)

// EnsureHostDir creates dir on the Docker host (mkdir -p) so bind mounts succeed.
// Modern Docker rejects bind sources that do not exist. The addon cannot mkdir on
// the host directly, so this uses a one-shot helper that bind-mounts the top-level
// directory of the path (via hostBindPaths) — any absolute host path works,
// including arbitrary USB/SSD mount points.
//
// Note: if the mount point is not present yet, mkdir -p may create a real directory
// that later shadows a removable drive mount — ensure the device is mounted first.
func (c *Client) EnsureHostDir(ctx context.Context, hostPath string) error {
	root, containerPath, err := hostBindPaths(hostPath)
	if err != nil {
		return err
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
//
// For any absolute path, the top-level directory is used as the bind root so
// operators can place caches on arbitrary mounts without an allowlist:
//
//	/scratch/build-cache/proj  → root=/scratch, rel=build-cache/proj
//	/media/usb0/ci-cache       → root=/media,   rel=usb0/ci-cache
func hostDirBindRoot(hostPath string) (root, rel string) {
	hostPath = path.Clean(hostPath)
	if hostPath == "/" {
		return "/", "."
	}
	rest := strings.TrimPrefix(hostPath, "/")
	i := strings.IndexByte(rest, '/')
	if i < 0 {
		return hostPath, "."
	}
	root = "/" + rest[:i]
	rel = rest[i+1:]
	if rel == "" {
		rel = "."
	}
	return root, rel
}
