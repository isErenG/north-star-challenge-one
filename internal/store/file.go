package store

import (
	"encoding/json"
	"os"
	"path/filepath"

	"kbo-review/internal/model"
)

// File writes one JSON document per job with private permissions. Writes go
// to a temporary file and are renamed into place so a crash never leaves a
// half-written job.
type File struct{ dir string }

// NewFile creates dir (0700) if needed.
func NewFile(dir string) (*File, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &File{dir: dir}, nil
}

func (f *File) Save(j *model.Job) error {
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	p := filepath.Join(f.dir, j.ID+".json")
	if err = os.WriteFile(p+".tmp", b, 0o600); err != nil {
		return err
	}
	return os.Rename(p+".tmp", p)
}

// LoadAll reads every job file. Unreadable files are skipped rather than
// failing startup.
func (f *File) LoadAll() ([]*model.Job, error) {
	paths, err := filepath.Glob(filepath.Join(f.dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var jobs []*model.Job
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var j model.Job
		if json.Unmarshal(b, &j) != nil {
			continue
		}
		jobs = append(jobs, &j)
	}
	return jobs, nil
}
