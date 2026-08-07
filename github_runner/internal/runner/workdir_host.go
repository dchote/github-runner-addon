package runner

import "context"

// workdirHost is the narrow seam for host/volume file ops used by the workdir pipeline.
// *docker.Client implements this; tests inject fakes.
type workdirHost interface {
	EnsureHostDir(ctx context.Context, hostPath string) error
	ReadVolumeFile(ctx context.Context, volumeName, relPath string) ([]byte, error)
	RemoveVolumeFiles(ctx context.Context, volumeName string, relPaths ...string) error
}

func (m *Manager) workdirHostOrDocker() workdirHost {
	if m.workdirHost != nil {
		return m.workdirHost
	}
	return m.Docker
}
