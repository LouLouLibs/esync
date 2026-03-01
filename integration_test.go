package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/louloulibs/esync/internal/config"
	"github.com/louloulibs/esync/internal/syncer"
	"github.com/louloulibs/esync/internal/watcher"
)

// TestLocalSyncIntegration verifies that the Syncer can rsync files between
// two local directories. This test requires rsync to be installed.
func TestLocalSyncIntegration(t *testing.T) {
	// Ensure rsync is available
	if _, err := os.Stat("/usr/bin/rsync"); err != nil {
		if _, err2 := os.Stat("/opt/homebrew/bin/rsync"); err2 != nil {
			t.Skip("rsync not found, skipping integration test")
		}
	}

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
	// Ensure rsync is available
	if _, err := os.Stat("/usr/bin/rsync"); err != nil {
		if _, err2 := os.Stat("/opt/homebrew/bin/rsync"); err2 != nil {
			t.Skip("rsync not found, skipping integration test")
		}
	}

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
	w, err := watcher.New(src, 100, nil, handler)
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
