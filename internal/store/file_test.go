package store

import (
	"os"
	"path/filepath"
	"testing"

	"kbo-review/internal/model"
)

func TestFileRoundTripAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "jobs")
	f, err := NewFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Save(&model.Job{ID: "abc", State: model.StateDone, Records: []model.Record{{ID: "1", Name: "A"}}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	jobs, err := f.LoadAll()
	if err != nil || len(jobs) != 1 || jobs[0].ID != "abc" || jobs[0].Records[0].Name != "A" {
		t.Fatalf("load: %v %+v", err, jobs)
	}
	info, _ := os.Stat(filepath.Join(dir, "abc.json"))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("job file permissions %v", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dir, "abc.json.tmp")); !os.IsNotExist(err) {
		t.Fatal("temporary file left behind")
	}
}
