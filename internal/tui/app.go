package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// View enum
// ---------------------------------------------------------------------------

type view int

const (
	viewDashboard view = iota
	viewLogs
)

// ---------------------------------------------------------------------------
// AppModel — root Bubbletea model
// ---------------------------------------------------------------------------

// AppModel is the root Bubbletea model that switches between the dashboard
// and log views.
type AppModel struct {
	dashboard  DashboardModel
	logView    LogViewModel
	current    view
	syncEvents chan SyncEvent
	logEntries chan LogEntry
}

// NewApp creates a new AppModel wired to the given local and remote paths.
func NewApp(local, remote string) *AppModel {
	return &AppModel{
		dashboard:  NewDashboard(local, remote),
		logView:    NewLogView(),
		current:    viewDashboard,
		syncEvents: make(chan SyncEvent, 64),
		logEntries: make(chan LogEntry, 64),
	}
}

// SyncEventChan returns a send-only channel for pushing sync events into
// the TUI from external code.
func (m *AppModel) SyncEventChan() chan<- SyncEvent {
	return m.syncEvents
}

// LogEntryChan returns a send-only channel for pushing log entries into
// the TUI from external code.
func (m *AppModel) LogEntryChan() chan<- LogEntry {
	return m.logEntries
}

// ---------------------------------------------------------------------------
// tea.Model interface
// ---------------------------------------------------------------------------

// Init returns a batch of the dashboard init command and the two channel
// listener commands.
func (m AppModel) Init() tea.Cmd {
	return tea.Batch(
		m.dashboard.Init(),
		m.listenSyncEvents(),
		m.listenLogEntries(),
	)
}

// Update delegates messages to the active view and handles global keys.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		// Global: quit from any view.
		switch msg.String() {
		case "q", "ctrl+c":
			// Let the current view handle it if it's filtering.
			if m.current == viewDashboard && m.dashboard.filtering {
				break
			}
			if m.current == viewLogs && m.logView.filtering {
				break
			}
			return m, tea.Quit
		case "l":
			// Toggle view (only when not filtering).
			if m.current == viewDashboard && !m.dashboard.filtering {
				m.current = viewLogs
				return m, nil
			}
			if m.current == viewLogs && !m.logView.filtering {
				m.current = viewDashboard
				return m, nil
			}
		}

	case SyncEventMsg:
		// Dispatch to dashboard and re-listen.
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, tea.Batch(cmd, m.listenSyncEvents())

	case LogEntryMsg:
		// Dispatch to log view and re-listen.
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		return m, tea.Batch(cmd, m.listenLogEntries())

	case tea.WindowSizeMsg:
		// Propagate to both views.
		m.dashboard, _ = m.dashboard.Update(msg)
		m.logView, _ = m.logView.Update(msg)
		return m, nil
	}

	// Delegate remaining messages to the active view.
	switch m.current {
	case viewDashboard:
		var cmd tea.Cmd
		m.dashboard, cmd = m.dashboard.Update(msg)
		return m, cmd
	case viewLogs:
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the currently active view.
func (m AppModel) View() string {
	switch m.current {
	case viewLogs:
		return m.logView.View()
	default:
		return m.dashboard.View()
	}
}

// ---------------------------------------------------------------------------
// Channel listeners
// ---------------------------------------------------------------------------

// listenSyncEvents returns a Cmd that blocks until a SyncEvent arrives on
// the channel, then wraps it as a SyncEventMsg.
func (m AppModel) listenSyncEvents() tea.Cmd {
	ch := m.syncEvents
	return func() tea.Msg {
		return SyncEventMsg(<-ch)
	}
}

// listenLogEntries returns a Cmd that blocks until a LogEntry arrives on
// the channel, then wraps it as a LogEntryMsg.
func (m AppModel) listenLogEntries() tea.Cmd {
	ch := m.logEntries
	return func() tea.Msg {
		return LogEntryMsg(<-ch)
	}
}
