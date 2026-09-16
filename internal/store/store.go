// Package store persists jobs. The in-memory registry and locking live in the
// jobs service; a Store only saves and loads.
package store

import "kbo-review/internal/model"

// Store saves jobs durably and loads them on startup.
type Store interface {
	Save(j *model.Job) error
	LoadAll() ([]*model.Job, error)
}

// Memory keeps jobs in a map. It is for tests only.
type Memory struct{ Jobs map[string]*model.Job }

func NewMemory() *Memory { return &Memory{Jobs: map[string]*model.Job{}} }

func (m *Memory) Save(j *model.Job) error {
	copy := *j
	m.Jobs[j.ID] = &copy
	return nil
}

func (m *Memory) LoadAll() ([]*model.Job, error) {
	out := make([]*model.Job, 0, len(m.Jobs))
	for _, j := range m.Jobs {
		out = append(out, j)
	}
	return out, nil
}
