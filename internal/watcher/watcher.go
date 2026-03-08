// Package watcher monitors a directory tree for file-system changes using
// fsnotify and debounces rapid events into a single callback.
package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ---------------------------------------------------------------------------
// EventHandler
// ---------------------------------------------------------------------------

// EventHandler is called after the debounce window closes.
type EventHandler func()

// ---------------------------------------------------------------------------
// Debouncer
// ---------------------------------------------------------------------------

// Debouncer batches rapid events into a single callback invocation.
// Each call to Trigger resets the timer; the callback fires only after
// the debounce interval elapses with no new triggers.
type Debouncer struct {
	interval time.Duration
	callback func()
	timer    *time.Timer
	mu       sync.Mutex
	stopped  bool
}

// NewDebouncer creates a Debouncer that will invoke callback after interval
// of inactivity following the most recent Trigger call.
func NewDebouncer(interval time.Duration, callback func()) *Debouncer {
	return &Debouncer{
		interval: interval,
		callback: callback,
	}
}

// Trigger resets the debounce timer. When the timer fires (after interval of
// inactivity), the callback is invoked.
func (d *Debouncer) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}

	if d.timer != nil {
		d.timer.Stop()
	}

	d.timer = time.AfterFunc(d.interval, func() {
		d.mu.Lock()
		stopped := d.stopped
		d.mu.Unlock()
		if !stopped {
			d.callback()
		}
	})
}

// Stop cancels any pending callback. After Stop returns, no further callbacks
// will be invoked even if Trigger is called again.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
	}
}

// ---------------------------------------------------------------------------
// Watcher
// ---------------------------------------------------------------------------

// Watcher monitors a directory tree for file-system changes using fsnotify.
// Events are debounced so that a burst of rapid changes results in a single
// call to the configured handler.
type Watcher struct {
	fsw       *fsnotify.Watcher
	debouncer *Debouncer
	path      string
	rootPath  string
	ignores   []string
	includes  []string
	done      chan struct{}
}

// New creates a Watcher for the given directory path. debounceMs sets the
// debounce interval in milliseconds (defaults to 500 if 0). ignores is a
// list of filepath.Match patterns to skip. handler is called after each
// debounced event batch.
func New(path string, debounceMs int, ignores []string, includes []string, handler EventHandler) (*Watcher, error) {
	if debounceMs <= 0 {
		debounceMs = 500
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		fsw:      fsw,
		path:     path,
		rootPath: absPath,
		ignores:  ignores,
		includes: includes,
		done:     make(chan struct{}),
	}

	w.debouncer = NewDebouncer(time.Duration(debounceMs)*time.Millisecond, handler)

	return w, nil
}

// Start adds the watched path recursively and launches the event loop in a
// background goroutine.
func (w *Watcher) Start() error {
	if err := w.addRecursive(w.path); err != nil {
		return err
	}

	go w.eventLoop()
	return nil
}

// Stop shuts down the watcher: cancels the debouncer, closes fsnotify, and
// waits for the event loop goroutine to exit.
func (w *Watcher) Stop() {
	w.debouncer.Stop()
	w.fsw.Close()
	<-w.done
}

// ---------------------------------------------------------------------------
// Private methods
// ---------------------------------------------------------------------------

// eventLoop reads fsnotify events and errors until the watcher is closed.
func (w *Watcher) eventLoop() {
	defer close(w.done)

	for {
		select {
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}

			// Only react to meaningful operations
			if !isRelevantOp(event.Op) {
				continue
			}

			if w.shouldIgnore(event.Name) {
				continue
			}

			if !w.shouldInclude(event.Name) {
				continue
			}

			// If a new directory was created, watch it recursively
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = w.addRecursive(event.Name)
				}
			}

			w.debouncer.Trigger()

		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			// Errors are logged but do not stop the loop.
		}
	}
}

// isRelevantOp returns true for file-system operations we care about.
// Chmod is included because touch(1) and some editors only update metadata,
// which fsnotify surfaces as Chmod on macOS (kqueue) and Linux (inotify).
func isRelevantOp(op fsnotify.Op) bool {
	return op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename|fsnotify.Chmod) != 0
}

// shouldIgnore checks the base name of path against all ignore patterns
// using filepath.Match.
func (w *Watcher) shouldIgnore(path string) bool {
	base := filepath.Base(path)
	for _, pattern := range w.ignores {
		if matched, _ := filepath.Match(pattern, base); matched {
			return true
		}
	}
	return false
}

// shouldInclude checks whether path falls within one of the configured include
// prefixes. If no includes are configured, every path is included. The method
// also returns true for ancestor directories of an include prefix (needed for
// traversal) and for the root path itself.
func (w *Watcher) shouldInclude(path string) bool {
	if len(w.includes) == 0 {
		return true
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}

	rel, err := filepath.Rel(w.rootPath, abs)
	if err != nil || rel == "." {
		return true
	}

	for _, inc := range w.includes {
		incClean := filepath.Clean(inc)
		// Path is the include prefix itself or is inside it
		if rel == incClean || strings.HasPrefix(rel, incClean+string(filepath.Separator)) {
			return true
		}
		// Path is an ancestor directory needed to reach the include prefix
		if strings.HasPrefix(incClean, rel+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// addRecursive walks the directory tree rooted at path and adds every
// directory to the fsnotify watcher. Individual files are not added
// because fsnotify watches directories for events on their contents.
func (w *Watcher) addRecursive(path string) error {
	return filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip entries we cannot stat
		}

		if w.shouldIgnore(p) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !w.shouldInclude(p) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return w.fsw.Add(p)
		}

		return nil
	})
}
