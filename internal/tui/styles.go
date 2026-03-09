// Package tui provides a terminal user interface for esync built on
// Bubbletea and Lipgloss.
package tui

import "github.com/charmbracelet/lipgloss"

// ---------------------------------------------------------------------------
// Lipgloss styles
// ---------------------------------------------------------------------------

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	statusSynced  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	statusSyncing = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	statusError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	focusedStyle  = lipgloss.NewStyle().Bold(true)
	helpKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true)
)
