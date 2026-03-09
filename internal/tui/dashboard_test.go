package tui

import "testing"

func TestMoveDownIntoChildren(t *testing.T) {
	m := NewDashboard("/tmp/local", "remote:/tmp")
	m.events = []SyncEvent{
		{File: "src/", Status: "synced", Files: []string{"src/a.go", "src/b.go"}, FileCount: 2},
		{File: "docs/", Status: "synced", Files: []string{"docs/readme.md"}, FileCount: 1},
	}
	m.expanded[0] = true

	filtered := m.filteredEvents()

	// Start at event 0, parent
	m.cursor = 0
	m.childCursor = -1

	// j → first child
	m.moveDown(filtered)
	if m.cursor != 0 || m.childCursor != 0 {
		t.Fatalf("expected cursor=0 child=0, got cursor=%d child=%d", m.cursor, m.childCursor)
	}

	// j → second child
	m.moveDown(filtered)
	if m.cursor != 0 || m.childCursor != 1 {
		t.Fatalf("expected cursor=0 child=1, got cursor=%d child=%d", m.cursor, m.childCursor)
	}

	// j → next event
	m.moveDown(filtered)
	if m.cursor != 1 || m.childCursor != -1 {
		t.Fatalf("expected cursor=1 child=-1, got cursor=%d child=%d", m.cursor, m.childCursor)
	}
}

func TestMoveUpFromChildren(t *testing.T) {
	m := NewDashboard("/tmp/local", "remote:/tmp")
	m.events = []SyncEvent{
		{File: "src/", Status: "synced", Files: []string{"src/a.go", "src/b.go"}, FileCount: 2},
		{File: "docs/", Status: "synced", Files: []string{"docs/readme.md"}, FileCount: 1},
	}
	m.expanded[0] = true

	filtered := m.filteredEvents()

	// Start at event 1
	m.cursor = 1
	m.childCursor = -1

	// k → last child of event 0
	m.moveUp(filtered)
	if m.cursor != 0 || m.childCursor != 1 {
		t.Fatalf("expected cursor=0 child=1, got cursor=%d child=%d", m.cursor, m.childCursor)
	}

	// k → first child
	m.moveUp(filtered)
	if m.cursor != 0 || m.childCursor != 0 {
		t.Fatalf("expected cursor=0 child=0, got cursor=%d child=%d", m.cursor, m.childCursor)
	}

	// k → parent
	m.moveUp(filtered)
	if m.cursor != 0 || m.childCursor != -1 {
		t.Fatalf("expected cursor=0 child=-1, got cursor=%d child=%d", m.cursor, m.childCursor)
	}
}

func TestMoveDownSkipsCollapsed(t *testing.T) {
	m := NewDashboard("/tmp/local", "remote:/tmp")
	m.events = []SyncEvent{
		{File: "src/", Status: "synced", Files: []string{"src/a.go"}, FileCount: 1},
		{File: "docs/", Status: "synced"},
	}
	// Not expanded — should skip children

	filtered := m.filteredEvents()
	m.cursor = 0
	m.childCursor = -1

	m.moveDown(filtered)
	if m.cursor != 1 || m.childCursor != -1 {
		t.Fatalf("expected cursor=1 child=-1, got cursor=%d child=%d", m.cursor, m.childCursor)
	}
}

func TestMoveDownAtEnd(t *testing.T) {
	m := NewDashboard("/tmp/local", "remote:/tmp")
	m.events = []SyncEvent{
		{File: "a.go", Status: "synced"},
	}
	filtered := m.filteredEvents()
	m.cursor = 0
	m.childCursor = -1

	m.moveDown(filtered)
	// Should stay at 0
	if m.cursor != 0 {
		t.Fatalf("expected cursor=0, got %d", m.cursor)
	}
}
