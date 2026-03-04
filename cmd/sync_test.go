package cmd

import (
	"testing"

	"github.com/louloulibs/esync/internal/syncer"
)

func TestGroupFilesByTopLevel_MultiFile(t *testing.T) {
	files := []syncer.FileEntry{
		{Name: "cmd/sync.go", Bytes: 100},
		{Name: "cmd/root.go", Bytes: 200},
		{Name: "main.go", Bytes: 50},
	}

	groups := groupFilesByTopLevel(files)

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}

	// First group: cmd/ with 2 files
	g := groups[0]
	if g.name != "cmd/" {
		t.Errorf("group[0].name = %q, want %q", g.name, "cmd/")
	}
	if g.count != 2 {
		t.Errorf("group[0].count = %d, want 2", g.count)
	}
	if len(g.files) != 2 {
		t.Fatalf("group[0].files has %d entries, want 2", len(g.files))
	}
	if g.files[0] != "cmd/sync.go" || g.files[1] != "cmd/root.go" {
		t.Errorf("group[0].files = %v, want [cmd/sync.go cmd/root.go]", g.files)
	}

	// Second group: root file
	g = groups[1]
	if g.name != "main.go" {
		t.Errorf("group[1].name = %q, want %q", g.name, "main.go")
	}
	if g.files != nil {
		t.Errorf("group[1].files should be nil for root file, got %v", g.files)
	}
}

func TestGroupFilesByTopLevel_SingleFileDir(t *testing.T) {
	files := []syncer.FileEntry{
		{Name: "internal/config/config.go", Bytes: 300},
	}

	groups := groupFilesByTopLevel(files)

	if len(groups) != 1 {
		t.Fatalf("got %d groups, want 1", len(groups))
	}

	g := groups[0]
	if g.name != "internal/config/config.go" {
		t.Errorf("name = %q, want full path", g.name)
	}
	if g.files != nil {
		t.Errorf("files should be nil for single-file dir, got %v", g.files)
	}
}
