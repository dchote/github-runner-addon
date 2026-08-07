package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/mount"
)

// ErrVolumeFileNotFound is returned when a path is missing inside a named volume.
var ErrVolumeFileNotFound = errors.New("volume file not found")

// ReadVolumeFile returns the contents of path inside a named volume (path relative to volume root).
func (c *Client) ReadVolumeFile(ctx context.Context, volumeName, relPath string) ([]byte, error) {
	relPath = strings.TrimPrefix(strings.TrimSpace(relPath), "/")
	if volumeName == "" || relPath == "" || strings.Contains(relPath, "..") {
		return nil, fmt.Errorf("invalid volume file path")
	}
	out, code, err := c.runHelper(ctx, []mount.Mount{{
		Type:   mount.TypeVolume,
		Source: volumeName,
		Target: "/vol",
	}}, []string{"cat", "/vol/" + relPath})
	if err != nil {
		return nil, err
	}
	if code != 0 {
		if code == 1 {
			return nil, fmt.Errorf("%w: %s in %s", ErrVolumeFileNotFound, relPath, volumeName)
		}
		return nil, fmt.Errorf("read %s from volume %s: exit %d: %s", relPath, volumeName, code, strings.TrimSpace(out))
	}
	return []byte(out), nil
}

// RemoveVolumeFiles deletes paths inside a named volume (relative to volume root).
func (c *Client) RemoveVolumeFiles(ctx context.Context, volumeName string, relPaths ...string) error {
	if volumeName == "" || len(relPaths) == 0 {
		return nil
	}
	args := []string{"rm", "-f"}
	for _, p := range relPaths {
		p = strings.TrimPrefix(strings.TrimSpace(p), "/")
		if p == "" || strings.Contains(p, "..") {
			return fmt.Errorf("invalid volume file path %q", p)
		}
		args = append(args, "/vol/"+p)
	}
	out, code, err := c.runHelper(ctx, []mount.Mount{{
		Type:   mount.TypeVolume,
		Source: volumeName,
		Target: "/vol",
	}}, args)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("remove from volume %s: exit %d: %s", volumeName, code, strings.TrimSpace(out))
	}
	return nil
}
