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

func normalizeVolumeRelPath(relPath string) (string, error) {
	relPath = strings.TrimPrefix(strings.TrimSpace(relPath), "/")
	if relPath == "" || strings.Contains(relPath, "..") {
		return "", fmt.Errorf("invalid volume file path")
	}
	return relPath, nil
}

// errIfVolumeAbsent maps VolumeExists into a ReadVolumeFile error. Missing volumes
// must not be auto-created (Engine would create an empty volume on helper mount).
func errIfVolumeAbsent(ok bool, err error, relPath, volumeName string) error {
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s in %s", ErrVolumeFileNotFound, relPath, volumeName)
	}
	return nil
}

// ReadVolumeFile returns the contents of path inside a named volume (relative to volume root).
// Uses CopyFromContainer (tar) — not container logs — so multiplex framing cannot corrupt JSON.
func (c *Client) ReadVolumeFile(ctx context.Context, volumeName, relPath string) ([]byte, error) {
	relPath, err := normalizeVolumeRelPath(relPath)
	if err != nil || volumeName == "" {
		return nil, fmt.Errorf("invalid volume file path")
	}
	ok, existsErr := c.VolumeExists(ctx, volumeName)
	if err := errIfVolumeAbsent(ok, existsErr, relPath, volumeName); err != nil {
		return nil, err
	}
	release, err := c.acquireHelper(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := c.EnsureImage(ctx, helperImage); err != nil {
		return nil, fmt.Errorf("helper image: %w", err)
	}
	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image:  helperImage,
		Cmd:    []string{"true"},
		Labels: helperContainerLabels(),
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

// WriteVolumeFile writes path inside a named volume (relative to volume root).
func (c *Client) WriteVolumeFile(ctx context.Context, volumeName, relPath string, data []byte) error {
	relPath, err := normalizeVolumeRelPath(relPath)
	if err != nil || volumeName == "" {
		return fmt.Errorf("invalid volume file path")
	}
	ok, existsErr := c.VolumeExists(ctx, volumeName)
	if err := errIfVolumeAbsent(ok, existsErr, relPath, volumeName); err != nil {
		return err
	}
	archive, err := tarRegularFile(relPath, data)
	if err != nil {
		return err
	}
	release, err := c.acquireHelper(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := c.EnsureImage(ctx, helperImage); err != nil {
		return fmt.Errorf("helper image: %w", err)
	}
	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image:  helperImage,
		Cmd:    []string{"true"},
		Labels: helperContainerLabels(),
	}, &container.HostConfig{
		Mounts: []mount.Mount{{
			Type:   mount.TypeVolume,
			Source: volumeName,
			Target: "/vol",
		}},
	}, nil, nil, "")
	if err != nil {
		return err
	}
	id := resp.ID
	defer func() { _ = c.removeByIDDetached(id) }()

	if err := c.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	statusCh, errCh := c.cli.ContainerWait(waitCtx, id, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case <-statusCh:
	}

	return c.cli.CopyToContainer(ctx, id, "/vol", archive, container.CopyToContainerOptions{})
}

// RemoveVolumeFiles deletes paths inside a named volume (relative to volume root).
func (c *Client) RemoveVolumeFiles(ctx context.Context, volumeName string, relPaths ...string) error {
	if volumeName == "" || len(relPaths) == 0 {
		return nil
	}
	ok, err := c.VolumeExists(ctx, volumeName)
	if err != nil {
		return err
	} else if !ok {
		return nil
	}
	args := []string{"rm", "-f"}
	for _, raw := range relPaths {
		p, err := normalizeVolumeRelPath(raw)
		if err != nil {
			return fmt.Errorf("invalid volume file path %q", raw)
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
