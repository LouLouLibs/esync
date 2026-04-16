package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/louloulibs/esync/internal/config"
	"github.com/louloulibs/esync/internal/syncer"
	"github.com/louloulibs/esync/internal/watcher"
)

// requireRsync skips the test unless a usable rsync (matching what
// syncer.rsyncBin selects and what syncer.CheckRsync validates) is
// available. Using CheckRsync as the source of truth keeps the
// skip-guard logic aligned with the production binary-selection path.
func requireRsync(t *testing.T) {
	t.Helper()
	if _, err := syncer.CheckRsync(); err != nil {
		t.Skipf("rsync unavailable: %v", err)
	}
}

// TestLocalSyncIntegration verifies that the Syncer can rsync files between
// two local directories. This test requires rsync to be installed.
func TestLocalSyncIntegration(t *testing.T) {
	requireRsync(t)

	// 1. Create two temp dirs (src, dst)
	src := t.TempDir()
	dst := t.TempDir()

	// 2. Write a test file to src
	testContent := "hello from integration test\n"
	testFile := filepath.Join(src, "testfile.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// 3. Create a Config pointing src -> dst with archive=true, progress=true
	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  src,
			Remote: dst,
		},
		Settings: config.Settings{
			Rsync: config.RsyncSettings{
				Archive:  true,
				Progress: true,
			},
		},
	}

	// 4. Create a Syncer, run it
	s := syncer.New(cfg)
	result, err := s.Run()

	// 5. Verify: no error, result.Success is true
	if err != nil {
		t.Fatalf("syncer.Run() returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected result.Success=true, got false; error: %s", result.ErrorMessage)
	}

	// 6. Verify: the file exists in dst with correct contents
	dstFile := filepath.Join(dst, "testfile.txt")
	got, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("failed to read synced file %s: %v", dstFile, err)
	}
	if string(got) != testContent {
		t.Errorf("synced file content mismatch:\n  got:  %q\n  want: %q", string(got), testContent)
	}
}

// TestWatcherTriggersSync verifies that the watcher detects a new file and
// triggers a sync from src to dst. This test requires rsync to be installed.
func TestWatcherTriggersSync(t *testing.T) {
	requireRsync(t)

	// 1. Create two temp dirs (src, dst)
	src := t.TempDir()
	dst := t.TempDir()

	// 2. Create Config and Syncer
	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  src,
			Remote: dst,
		},
		Settings: config.Settings{
			Rsync: config.RsyncSettings{
				Archive:  true,
				Progress: true,
			},
		},
	}
	s := syncer.New(cfg)

	// 3. Create watcher with handler that runs syncer and signals a channel
	synced := make(chan struct{}, 1)
	handler := func() {
		_, _ = s.Run()
		select {
		case synced <- struct{}{}:
		default:
		}
	}

	// Use short debounce (100ms) for fast tests
	w, err := watcher.New(src, 100, nil, nil, handler)
	if err != nil {
		t.Fatalf("watcher.New() failed: %v", err)
	}

	// 4. Start watcher
	if err := w.Start(); err != nil {
		t.Fatalf("watcher.Start() failed: %v", err)
	}
	defer w.Stop()

	// 5. Wait 200ms for watcher to settle
	time.Sleep(200 * time.Millisecond)

	// 6. Write a file to src
	testContent := "watcher triggered sync\n"
	testFile := filepath.Join(src, "watched.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// 7. Wait for signal (with 5s timeout)
	select {
	case <-synced:
		// success - sync was triggered
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for watcher to trigger sync")
	}

	// 8. Verify: file was synced to dst with correct contents
	dstFile := filepath.Join(dst, "watched.txt")
	got, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("failed to read synced file %s: %v", dstFile, err)
	}
	if string(got) != testContent {
		t.Errorf("synced file content mismatch:\n  got:  %q\n  want: %q", string(got), testContent)
	}
}

// TestIssue14IgnoreHonoredUnderInclude reproduces issue #14: when `include`
// contains a directory prefix and `ignore` lists a path that lives inside
// that prefix, `sync` must NOT transfer the ignored path (matching what
// `check` reports).
//
// Uses rsync --dry-run so it does not actually copy bytes; parses rsync's
// transfer list for the ignored path.
func TestIssue14IgnoreHonoredUnderInclude(t *testing.T) {
	requireRsync(t)

	src := t.TempDir()
	dst := t.TempDir()

	// Build the fixture tree:
	//   src/worktree/env/.venv/big.bin     (should NOT be transferred)
	//   src/worktree/env/keep.txt          (should be transferred)
	venvDir := filepath.Join(src, "worktree", "env", ".venv")
	if err := os.MkdirAll(venvDir, 0755); err != nil {
		t.Fatalf("mkdir venv: %v", err)
	}
	if err := os.WriteFile(filepath.Join(venvDir, "big.bin"), []byte("should be ignored"), 0644); err != nil {
		t.Fatalf("write big.bin: %v", err)
	}
	keepPath := filepath.Join(src, "worktree", "env", "keep.txt")
	if err := os.WriteFile(keepPath, []byte("should be synced"), 0644); err != nil {
		t.Fatalf("write keep.txt: %v", err)
	}

	cfg := &config.Config{
		Sync: config.SyncSection{
			Local:  src,
			Remote: dst,
		},
		Settings: config.Settings{
			Include: []string{"worktree/"},
			Ignore:  []string{".venv/", "**/.venv/"},
			Rsync: config.RsyncSettings{
				Archive:  true,
				Progress: true,
			},
		},
	}

	s := syncer.New(cfg)
	s.DryRun = true

	result, err := s.Run()
	if err != nil {
		t.Fatalf("syncer.Run(): %v\n%s", err, result.ErrorMessage)
	}
	if !result.Success {
		t.Fatalf("syncer did not succeed: %s", result.ErrorMessage)
	}

	// The .venv/ subtree must not appear in the transferred file list.
	for _, f := range result.Files {
		if strings.Contains(f.Name, ".venv") {
			t.Errorf("issue #14 regression: .venv path appeared in transfer list: %q\nall files: %v", f.Name, result.Files)
		}
	}

	// Sanity: keep.txt must appear (otherwise the filter over-excluded).
	foundKeep := false
	for _, f := range result.Files {
		if strings.HasSuffix(f.Name, "keep.txt") {
			foundKeep = true
			break
		}
	}
	if !foundKeep {
		t.Errorf("worktree/env/keep.txt was filtered out by mistake; files: %v", result.Files)
	}
}
