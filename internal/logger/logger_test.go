package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJSONLogger(t *testing.T) {
	// Create a temp file for the log
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	lg, err := New(logPath, "json")
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer lg.Close()

	// Write an info entry with fields
	lg.Info("synced", map[string]interface{}{
		"file": "main.go",
		"size": 2150,
	})

	// Read the file contents
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	line := strings.TrimSpace(string(data))
	if line == "" {
		t.Fatal("log file is empty")
	}

	// Parse as JSON
	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		t.Fatalf("log entry is not valid JSON: %v\nline: %s", err, line)
	}

	// Verify required fields
	if entry["level"] != "info" {
		t.Errorf("expected level=info, got %v", entry["level"])
	}
	if entry["event"] != "synced" {
		t.Errorf("expected event=synced, got %v", entry["event"])
	}
	if entry["file"] != "main.go" {
		t.Errorf("expected file=main.go, got %v", entry["file"])
	}
	// JSON numbers are float64 by default
	if entry["size"] != float64(2150) {
		t.Errorf("expected size=2150, got %v", entry["size"])
	}
	if _, ok := entry["time"]; !ok {
		t.Error("expected time field to be present")
	}
}

func TestTextLogger(t *testing.T) {
	// Create a temp file for the log
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	lg, err := New(logPath, "text")
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer lg.Close()

	// Write an info entry with fields
	lg.Info("synced", map[string]interface{}{
		"file": "main.go",
		"size": 2150,
	})

	// Read the file contents
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	line := strings.TrimSpace(string(data))
	if line == "" {
		t.Fatal("log file is empty")
	}

	// Verify text format contains INF and event name
	if !strings.Contains(line, "INF") {
		t.Errorf("expected line to contain 'INF', got: %s", line)
	}
	if !strings.Contains(line, "synced") {
		t.Errorf("expected line to contain 'synced', got: %s", line)
	}
	if !strings.Contains(line, "file=main.go") {
		t.Errorf("expected line to contain 'file=main.go', got: %s", line)
	}
}

func TestDefaultFormatIsText(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	lg, err := New(logPath, "")
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer lg.Close()

	if lg.format != "text" {
		t.Errorf("expected default format to be 'text', got %q", lg.format)
	}
}

func TestWarnLevel(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	lg, err := New(logPath, "text")
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer lg.Close()

	lg.Warn("disk_low", map[string]interface{}{
		"pct": 95,
	})

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	line := strings.TrimSpace(string(data))
	if !strings.Contains(line, "WRN") {
		t.Errorf("expected line to contain 'WRN', got: %s", line)
	}
	if !strings.Contains(line, "disk_low") {
		t.Errorf("expected line to contain 'disk_low', got: %s", line)
	}
}

func TestErrorLevel(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	lg, err := New(logPath, "json")
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer lg.Close()

	lg.Error("connection_failed", map[string]interface{}{
		"host": "example.com",
	})

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	var entry map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &entry); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}

	if entry["level"] != "error" {
		t.Errorf("expected level=error, got %v", entry["level"])
	}
}

func TestDebugLevel(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	lg, err := New(logPath, "text")
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	defer lg.Close()

	lg.Debug("trace_check", nil)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	line := strings.TrimSpace(string(data))
	if !strings.Contains(line, "DBG") {
		t.Errorf("expected line to contain 'DBG', got: %s", line)
	}
	if !strings.Contains(line, "trace_check") {
		t.Errorf("expected line to contain 'trace_check', got: %s", line)
	}
}
