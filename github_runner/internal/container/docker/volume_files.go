package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
)

// ErrVolumeFileNotFound is returned when a path is missing inside a named volume.
var ErrVolumeFileNotFound = errors.New("volume file not found")

// ReadVolumeFile returns the contents of path inside a named volume (relative to volume root).
// Uses CopyFromContainer (tar) — not container logs — so multiplex framing cannot corrupt JSON.
func (c *Client) ReadVolumeFile(ctx context.Context, volumeName, relPath string) ([]byte, error) {
	relPath = strings.TrimPrefix(strings.TrimSpace(relPath), "/")
	if volumeName == "" || relPath == "" || strings.Contains(relPath, "..") {
		return nil, fmt.Errorf("invalid volume file path")
	}
	if err := c.EnsureImage(ctx, helperImage); err != nil {
		return nil, fmt.Errorf("helper image: %w", err)
	}
	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image: helperImage,
		Cmd:   []string{"true"},
	}, &container.HostConfig{
		Mounts: []mount.Mount{{
			Type:   mount.TypeVolume,
			Source: volumeName,
			Target: "/vol",
		}},
	}, nil, nil, "")
	if err != nil {
		return nil, err
	}
	id := resp.ID
	defer func() { _ = c.removeByIDDetached(id) }()

	if err := c.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return nil, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	statusCh, errCh := c.cli.ContainerWait(waitCtx, id, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return nil, err
		}
	case <-statusCh:
	}

	reader, _, err := c.cli.CopyFromContainer(ctx, id, "/vol/"+relPath)
	if err != nil {
		if client.IsErrNotFound(err) {
			return nil, fmt.Errorf("%w: %s in %s", ErrVolumeFileNotFound, relPath, volumeName)
		}
		return nil, err
	}
	defer reader.Close()
	return readTarRegularFile(reader, ErrVolumeFileNotFound, relPath+" in "+volumeName)
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
