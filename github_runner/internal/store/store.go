package store

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

var ErrNotFound = errors.New("runner not found")
var ErrConflict = errors.New("runner name already exists")

const SchemaVersion = 3

// CacheConfig is an optional persistent cache mount for job tooling / build caches.
type CacheConfig struct {
	Enabled    bool   `json:"enabled"`
	Type       string `json:"type,omitempty"`        // volume | bind; default volume when enabled
	VolumeName string `json:"volume_name,omitempty"` // named volume; empty → gha-runner-<name>-cache
	HostPath   string `json:"host_path,omitempty"`   // Docker host path when type=bind
	Target     string `json:"target,omitempty"`      // container path; default /cache
	ReadOnly   bool   `json:"read_only,omitempty"`
}

// Runner is the persisted expected configuration (registration token is not stored here).
type Runner struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	URL             string            `json:"url"`
	Scope           string            `json:"scope"` // repo | org
	Labels          []string          `json:"labels"`
	ContainerName   string            `json:"container_name"`
	VolumeName      string            `json:"volume_name"`
	Image           string            `json:"image"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at,omitempty"`
	CPULimit        float64           `json:"cpu_limit,omitempty"`       // CPU cores; 0 = unlimited (Docker NanoCPUs = cores * 1e9)
	MemoryLimitMB   int64             `json:"memory_limit_mb,omitempty"` // MiB; 0 = unlimited
	ExtraEnv        map[string]string `json:"extra_env,omitempty"`
	NetworkMode     string            `json:"network_mode,omitempty"`
	MountDockerSock *bool             `json:"mount_docker_sock,omitempty"` // nil = use global default
	Cache           *CacheConfig      `json:"cache,omitempty"`
	PersistWorkdir  bool              `json:"persist_workdir,omitempty"`   // named volume at /work (ignored when WorkdirHostPath is set)
	WorkdirHostPath string            `json:"workdir_host_path,omitempty"` // same-path host bind for sibling docker -v $GITHUB_WORKSPACE
}

type fileData struct {
	Version int      `json:"version"`
	Runners []Runner `json:"runners"`
}

// Store persists runners.json with a mutex.
type Store struct {
	path string
	mu   sync.Mutex
}

func New(path string) *Store {
	return &Store{path: path}
}

// Path returns the store file path (for health checks).
func (s *Store) Path() string {
	return s.path
}

func (s *Store) load() (fileData, error) {
	var data fileData
	b, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return fileData{Version: SchemaVersion, Runners: []Runner{}}, nil
	}
	if err != nil {
		return data, err
	}
	if len(b) == 0 {
		return fileData{Version: SchemaVersion, Runners: []Runner{}}, nil
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return data, err
	}
	if data.Runners == nil {
		data.Runners = []Runner{}
	}
	if data.Version == 0 {
		data.Version = 1
	}
	return data, nil
}

func (s *Store) save(data fileData) error {
	data.Version = SchemaVersion
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Readable reports whether the store file can be loaded.
func (s *Store) Readable() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.load()
	return err
}

func (s *Store) List() ([]Runner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]Runner, len(data.Runners))
	copy(out, data.Runners)
	return out, nil
}

func (s *Store) Get(id string) (Runner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.load()
	if err != nil {
		return Runner{}, err
	}
	for _, r := range data.Runners {
		if r.ID == id {
			return r, nil
		}
	}
	return Runner{}, ErrNotFound
}

func (s *Store) Add(r Runner) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.load()
	if err != nil {
		return err
	}
	for _, existing := range data.Runners {
		if existing.Name == r.Name || existing.ContainerName == r.ContainerName {
			return ErrConflict
		}
	}
	data.Runners = append(data.Runners, r)
	return s.save(data)
}

// Update replaces an existing runner record by id.
func (s *Store) Update(r Runner) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.load()
	if err != nil {
		return err
	}
	for i, existing := range data.Runners {
		if existing.ID != r.ID {
			continue
		}
		for _, other := range data.Runners {
			if other.ID == r.ID {
				continue
			}
			if other.Name == r.Name || other.ContainerName == r.ContainerName {
				return ErrConflict
			}
		}
		r.UpdatedAt = time.Now().UTC()
		data.Runners[i] = r
		return s.save(data)
	}
	return ErrNotFound
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.load()
	if err != nil {
		return err
	}
	next := make([]Runner, 0, len(data.Runners))
	found := false
	for _, r := range data.Runners {
		if r.ID == id {
			found = true
			continue
		}
		next = append(next, r)
	}
	if !found {
		return ErrNotFound
	}
	data.Runners = next
	return s.save(data)
}
