package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/pkg/stdcopy"
)

// Lightweight image for one-shot volume file ops (read/clear .runner).
const volumeHelperImage = "alpine:3.20"

// ReadVolumeFile returns the contents of path inside a named volume (path relative to volume root).
func (c *Client) ReadVolumeFile(ctx context.Context, volumeName, relPath string) ([]byte, error) {
	relPath = strings.TrimPrefix(strings.TrimSpace(relPath), "/")
	if volumeName == "" || relPath == "" || strings.Contains(relPath, "..") {
		return nil, fmt.Errorf("invalid volume file path")
	}
	out, code, err := c.runVolumeHelper(ctx, volumeName, []string{"cat", "/vol/" + relPath})
	if err != nil {
		return nil, err
	}
	if code != 0 {
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
	out, code, err := c.runVolumeHelper(ctx, volumeName, args)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("remove from volume %s: exit %d: %s", volumeName, code, strings.TrimSpace(out))
	}
	return nil
}

func (c *Client) runVolumeHelper(ctx context.Context, volumeName string, cmd []string) (string, int, error) {
	if err := c.EnsureImage(ctx, volumeHelperImage); err != nil {
		return "", -1, fmt.Errorf("volume helper image: %w", err)
	}
	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image: volumeHelperImage,
		Cmd:   cmd,
	}, &container.HostConfig{
		Mounts: []mount.Mount{{
			Type:   mount.TypeVolume,
			Source: volumeName,
			Target: "/vol",
		}},
	}, nil, nil, "")
	if err != nil {
		return "", -1, err
	}
	return c.waitHelperLogs(ctx, resp.ID)
}

func (c *Client) waitHelperLogs(ctx context.Context, id string) (string, int, error) {
	defer func() { _ = c.removeByIDDetached(id) }()

	if err := c.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return "", -1, err
	}

	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	statusCh, errCh := c.cli.ContainerWait(waitCtx, id, container.WaitConditionNotRunning)
	var code int64
	select {
	case err := <-errCh:
		if err != nil {
			return "", -1, err
		}
	case st := <-statusCh:
		code = st.StatusCode
		if st.Error != nil && st.Error.Message != "" {
			return "", int(code), fmt.Errorf("%s", st.Error.Message)
		}
	}

	logs, err := c.cli.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
	})
	if err != nil {
		if code == 0 {
			return "", 0, nil
		}
		return "", int(code), err
	}
	defer logs.Close()
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, io.Discard, logs); err != nil {
		_, _ = io.Copy(&buf, logs)
	}
	return buf.String(), int(code), nil
}
