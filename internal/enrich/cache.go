package enrich

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Cache stores source responses so re-running a file costs nothing.
type Cache interface {
	Get(source, key string, out any) bool
	Put(source, key string, v any)
}

// FileCache keeps one JSON file per entry under dir/<source>/<sha256>.json.
type FileCache struct {
	dir string
	ttl time.Duration
}

type entry struct {
	Stored time.Time       `json:"stored"`
	Data   json.RawMessage `json:"data"`
}

func NewFileCache(dir string, ttl time.Duration) *FileCache {
	return &FileCache{dir: dir, ttl: ttl}
}

func (c *FileCache) path(source, key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.dir, source, hex.EncodeToString(sum[:])+".json")
}

func (c *FileCache) Get(source, key string, out any) bool {
	b, err := os.ReadFile(c.path(source, key))
	if err != nil {
		return false
	}
	var e entry
	if json.Unmarshal(b, &e) != nil || time.Since(e.Stored) > c.ttl {
		return false
	}
	return json.Unmarshal(e.Data, out) == nil
}

func (c *FileCache) Put(source, key string, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	b, _ := json.Marshal(entry{Stored: time.Now(), Data: data})
	p := c.path(source, key)
	if os.MkdirAll(filepath.Dir(p), 0o700) != nil {
		return
	}
	if os.WriteFile(p+".tmp", b, 0o600) == nil {
		_ = os.Rename(p+".tmp", p)
	}
}

// NoCache disables caching (tests).
type NoCache struct{}

func (NoCache) Get(string, string, any) bool { return false }
func (NoCache) Put(string, string, any)      {}
