package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/pkg/stdcopy"
)

// Lightweight image for one-shot host/volume helpers.
const helperImage = "alpine:3.20"

func (c *Client) runHelper(ctx context.Context, mounts []mount.Mount, cmd []string) (string, int, error) {
	if c.helperSem != nil {
		if err := c.helperSem.Acquire(ctx, 1); err != nil {
			return "", -1, err
		}
		defer c.helperSem.Release(1)
	}
	if err := c.EnsureImage(ctx, helperImage); err != nil {
		return "", -1, fmt.Errorf("helper image: %w", err)
	}
	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image: helperImage,
		Cmd:   cmd,
	}, &container.HostConfig{
		Mounts: mounts,
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
