package docker

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

// ErrHostFileNotFound is returned when a path is missing on the Docker host.
var ErrHostFileNotFound = errors.New("host file not found")

// ErrContainerFileNotFound is returned when a path is missing inside a container.
var ErrContainerFileNotFound = errors.New("container file not found")

// hostBindPaths maps an absolute Docker-host path to a helper bind root and
// the corresponding path inside the helper (under /host).
func hostBindPaths(hostPath string) (root, containerPath string, err error) {
	hostPath, err = sanitizeHostPath(hostPath)
	if err != nil {
		return "", "", err
	}
	root, rel := hostDirBindRoot(hostPath)
	containerPath = path.Join("/host", rel)
	if rel == "" || rel == "." {
		containerPath = "/host"
	}
	return root, containerPath, nil
}

// WriteHostFile writes data to an absolute path on the Docker host (mkdir -p parent).
// mode is applied with chmod (e.g. 0o755 for scripts, 0o666 for shared status JSON).
func (c *Client) WriteHostFile(ctx context.Context, hostPath string, data []byte, mode os.FileMode) error {
	root, containerPath, err := hostBindPaths(hostPath)
	if err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	parent := path.Dir(containerPath)
	b64 := base64.StdEncoding.EncodeToString(data)
	script := fmt.Sprintf(
		`mkdir -p %q && printf '%%s' %q | base64 -d > %q && chmod %04o %q`,
		parent, b64, containerPath, mode.Perm(), containerPath,
	)
	out, code, err := c.runHelper(ctx, []mount.Mount{{
		Type:   mount.TypeBind,
		Source: root,
		Target: "/host",
	}}, []string{"sh", "-c", script})
	if err != nil {
		return fmt.Errorf("write host file %s: %w", hostPath, err)
	}
	if code != 0 {
		return fmt.Errorf("write host file %s: exit %d: %s", hostPath, code, strings.TrimSpace(out))
	}
	return nil
}

// ReadHostFile returns the contents of an absolute path on the Docker host.
func (c *Client) ReadHostFile(ctx context.Context, hostPath string) ([]byte, error) {
	root, containerPath, err := hostBindPaths(hostPath)
	if err != nil {
		return nil, err
	}
	script := fmt.Sprintf(
		`if [ ! -f %q ]; then exit 2; fi; cat %q`,
		containerPath, containerPath,
	)
	out, code, err := c.runHelper(ctx, []mount.Mount{{
		Type:   mount.TypeBind,
		Source: root,
		Target: "/host",
	}}, []string{"sh", "-c", script})
	if err != nil {
		return nil, fmt.Errorf("read host file %s: %w", hostPath, err)
	}
	if code == 2 {
		return nil, fmt.Errorf("%w: %s", ErrHostFileNotFound, hostPath)
	}
	if code != 0 {
		return nil, fmt.Errorf("read host file %s: exit %d: %s", hostPath, code, strings.TrimSpace(out))
	}
	return []byte(out), nil
}

// ChmodHostPath sets the permission bits on an absolute Docker-host path.
func (c *Client) ChmodHostPath(ctx context.Context, hostPath string, mode os.FileMode) error {
	root, containerPath, err := hostBindPaths(hostPath)
	if err != nil {
		return err
	}
	script := fmt.Sprintf(`chmod %04o %q`, mode.Perm(), containerPath)
	out, code, err := c.runHelper(ctx, []mount.Mount{{
		Type:   mount.TypeBind,
		Source: root,
		Target: "/host",
	}}, []string{"sh", "-c", script})
	if err != nil {
		return fmt.Errorf("chmod host path %s: %w", hostPath, err)
	}
	if code != 0 {
		return fmt.Errorf("chmod host path %s: exit %d: %s", hostPath, code, strings.TrimSpace(out))
	}
	return nil
}

// ReadContainerFile returns a regular file from a container (name or ID) via CopyFromContainer.
func (c *Client) ReadContainerFile(ctx context.Context, containerRef, absPath string) ([]byte, error) {
	absPath = strings.TrimSpace(absPath)
	if containerRef == "" || absPath == "" || !strings.HasPrefix(absPath, "/") || strings.Contains(absPath, "..") {
		return nil, fmt.Errorf("invalid container file path")
	}
	reader, _, err := c.cli.CopyFromContainer(ctx, containerRef, absPath)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, fmt.Errorf("%w: %s in %s", ErrContainerFileNotFound, absPath, containerRef)
		}
		return nil, err
	}
	defer reader.Close()
	return readTarRegularFile(reader, ErrContainerFileNotFound, absPath+" in "+containerRef)
}
