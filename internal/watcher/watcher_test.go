package watcher

import (
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
