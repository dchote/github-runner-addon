package runner

import (
	"context"
	"os"
)

// workdirHost is the narrow seam for host/volume file ops used by the workdir pipeline.
// *docker.Client implements this; tests inject fakes.
type workdirHost interface {
	EnsureHostDir(ctx context.Context, hostPath string) error
	WriteHostFile(ctx context.Context, hostPath string, data []byte, mode os.FileMode) error
	ReadHostFile(ctx context.Context, hostPath string) ([]byte, error)
	ChmodHostPath(ctx context.Context, hostPath string, mode os.FileMode) error
	ReadVolumeFile(ctx context.Context, volumeName, relPath string) ([]byte, error)
	RemoveVolumeFiles(ctx context.Context, volumeName string, relPaths ...string) error
}

func (m *Manager) workdirHostOrDocker() workdirHost {
	if m.workdirHost != nil {
		return m.workdirHost
	}
	return m.Docker
}
