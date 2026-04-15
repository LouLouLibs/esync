package watcher

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. TestDebouncerBatchesEvents — rapid events produce exactly one callback
// ---------------------------------------------------------------------------
func TestDebouncerBatchesEvents(t *testing.T) {
	var count atomic.Int64

	d := NewDebouncer(100*time.Millisecond, func() {
		count.Add(1)
	})
	defer d.Stop()

	// Fire 5 events rapidly, 10ms apart
	for i := 0; i < 5; i++ {
		d.Trigger()
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debounce window to expire plus margin
	time.Sleep(200 * time.Millisecond)

	got := count.Load()
	if got != 1 {
		t.Errorf("callback fired %d times, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// 2. TestDebouncerSeparateEvents — two events separated by more than the
//    debounce interval should fire the callback twice
// ---------------------------------------------------------------------------
func TestDebouncerSeparateEvents(t *testing.T) {
	var count atomic.Int64

	d := NewDebouncer(50*time.Millisecond, func() {
		count.Add(1)
	})
	defer d.Stop()

	// First event
	d.Trigger()
	// Wait for the debounce to fire
	time.Sleep(150 * time.Millisecond)

	// Second event
	d.Trigger()
	// Wait for the debounce to fire
	time.Sleep(150 * time.Millisecond)

	got := count.Load()
	if got != 2 {
		t.Errorf("callback fired %d times, want 2", got)
	}
}

// ---------------------------------------------------------------------------
// 3. TestDebouncerStopCancelsPending — Stop prevents a pending callback
// ---------------------------------------------------------------------------
func TestDebouncerStopCancelsPending(t *testing.T) {
	var count atomic.Int64

	d := NewDebouncer(100*time.Millisecond, func() {
		count.Add(1)
	})

	d.Trigger()
	// Stop before the debounce interval elapses
	time.Sleep(20 * time.Millisecond)
	d.Stop()

	// Wait past the debounce interval
	time.Sleep(200 * time.Millisecond)

	got := count.Load()
	if got != 0 {
		t.Errorf("callback fired %d times after Stop, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// 4. TestShouldIgnore — verify ignore pattern matching
// ---------------------------------------------------------------------------
func TestShouldIgnore(t *testing.T) {
	w := &Watcher{
		ignores: []string{".git", "*.tmp", "node_modules"},
	}

	tests := []struct {
		path   string
		expect bool
	}{
		{"/project/.git", true},
		{"/project/foo.tmp", true},
		{"/project/node_modules", true},
		{"/project/main.go", false},
		{"/project/src/app.go", false},
	}

	for _, tt := range tests {
		got := w.shouldIgnore(tt.path)
		if got != tt.expect {
			t.Errorf("shouldIgnore(%q) = %v, want %v", tt.path, got, tt.expect)
		}
	}
}

// ---------------------------------------------------------------------------
// 5. TestShouldInclude — verify include prefix matching
// ---------------------------------------------------------------------------
func TestShouldInclude(t *testing.T) {
	w := &Watcher{
		rootPath: "/project",
		includes: []string{"src", "docs/api"},
	}

	tests := []struct {
		path   string
		expect bool
	}{
		{"/project/src/main.go", true},
		{"/project/src/pkg/util.go", true},
		{"/project/docs/api/readme.md", true},
		{"/project/docs", true},           // ancestor of docs/api
		{"/project/tmp/cache.bin", false},
		{"/project/build/out.o", false},
		{"/project", true},                // root always included
	}

	for _, tt := range tests {
		got := w.shouldInclude(tt.path)
		if got != tt.expect {
			t.Errorf("shouldInclude(%q) = %v, want %v", tt.path, got, tt.expect)
		}
	}
}

// ---------------------------------------------------------------------------
// 6. TestShouldIncludeEmptyMeansAll — empty includes means include everything
// ---------------------------------------------------------------------------
func TestShouldIncludeEmptyMeansAll(t *testing.T) {
	w := &Watcher{
		rootPath: "/project",
		includes: nil,
	}

	tests := []struct {
		path   string
		expect bool
	}{
		{"/project/anything/at/all.go", true},
		{"/project/tmp/cache.bin", true},
	}

	for _, tt := range tests {
		got := w.shouldInclude(tt.path)
		if got != tt.expect {
			t.Errorf("shouldInclude(%q) = %v, want %v", tt.path, got, tt.expect)
		}
	}
}

// ---------------------------------------------------------------------------
// 7. TestFindBrokenSymlinks — detects broken symlinks in a directory
// ---------------------------------------------------------------------------
func TestFindBrokenSymlinks(t *testing.T) {
	dir := t.TempDir()

	// Create a valid file
	os.WriteFile(filepath.Join(dir, "good.txt"), []byte("ok"), 0644)

	// Create a broken symlink
	os.Symlink("/nonexistent/target", filepath.Join(dir, "bad.txt"))

	// Create a valid symlink
	os.Symlink(filepath.Join(dir, "good.txt"), filepath.Join(dir, "also-good.txt"))

	broken := findBrokenSymlinks(dir)

	if len(broken) != 1 {
		t.Fatalf("findBrokenSymlinks found %d, want 1", len(broken))
	}
	if broken[0].Target != "/nonexistent/target" {
		t.Errorf("target = %q, want %q", broken[0].Target, "/nonexistent/target")
	}
	if filepath.Base(broken[0].Path) != "bad.txt" {
		t.Errorf("path base = %q, want %q", filepath.Base(broken[0].Path), "bad.txt")
	}
}

// ---------------------------------------------------------------------------
// 8. TestAddRecursiveSkipsBrokenSymlinks — watcher starts despite broken symlinks
// ---------------------------------------------------------------------------
func TestAddRecursiveSkipsBrokenSymlinks(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	os.Mkdir(sub, 0755)

	// Create a broken symlink inside subdir
	os.Symlink("/nonexistent/target", filepath.Join(sub, "broken.csv"))

	w, err := New(dir, 100, nil, nil, func() {})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer w.Stop()

	// Start should succeed despite broken symlinks
	if err := w.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if len(w.BrokenSymlinks) != 1 {
		t.Fatalf("BrokenSymlinks = %d, want 1", len(w.BrokenSymlinks))
	}
	if w.BrokenSymlinks[0].Target != "/nonexistent/target" {
		t.Errorf("target = %q, want %q", w.BrokenSymlinks[0].Target, "/nonexistent/target")
	}
	if filepath.Base(w.BrokenSymlinks[0].Path) != "broken.csv" {
		t.Errorf("path base = %q, want %q", filepath.Base(w.BrokenSymlinks[0].Path), "broken.csv")
	}
}
