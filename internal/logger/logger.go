package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
)

// Logger writes structured log entries to a file in either JSON or text format.
type Logger struct {
	file   *os.File
	format string // "json" or "text"
	mu     sync.Mutex
}

// New opens (or creates) a log file at path for append-only writing and returns
// a Logger. The format parameter must be "json" or "text"; an empty string
// defaults to "text".
func New(path string, format string) (*Logger, error) {
	if format == "" {
		format = "text"
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("logger: open %s: %w", path, err)
	}

	return &Logger{
		file:   f,
		format: format,
	}, nil
}

// Close closes the underlying log file.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// Info logs an info-level entry.
func (l *Logger) Info(event string, fields map[string]interface{}) {
	l.log("info", event, fields)
}

// Warn logs a warn-level entry.
func (l *Logger) Warn(event string, fields map[string]interface{}) {
	l.log("warn", event, fields)
}

// Error logs an error-level entry.
func (l *Logger) Error(event string, fields map[string]interface{}) {
	l.log("error", event, fields)
}

// Debug logs a debug-level entry.
func (l *Logger) Debug(event string, fields map[string]interface{}) {
	l.log("debug", event, fields)
}

// levelTag maps internal level names to short text-format tags.
var levelTag = map[string]string{
	"info":  "INF",
	"warn":  "WRN",
	"error": "ERR",
	"debug": "DBG",
}

// log writes a single log entry in the configured format.
func (l *Logger) log(level, event string, fields map[string]interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	ts := time.Now().Format("15:04:05")

	switch l.format {
	case "json":
		l.writeJSON(ts, level, event, fields)
	default:
		l.writeText(ts, level, event, fields)
	}
}

// writeJSON writes a single JSON log line.
func (l *Logger) writeJSON(ts, level, event string, fields map[string]interface{}) {
	entry := make(map[string]interface{}, len(fields)+3)
	entry["time"] = ts
	entry["level"] = level
	entry["event"] = event
	for k, v := range fields {
		entry[k] = v
	}

	data, err := json.Marshal(entry)
	if err != nil {
		// Best-effort fallback: write the error itself.
		fmt.Fprintf(l.file, `{"time":%q,"level":"error","event":"log_marshal_error","error":%q}`+"\n", ts, err.Error())
		return
	}
	l.file.Write(data)
	l.file.Write([]byte("\n"))
}

// writeText writes a single text log line in the format:
//
//	15:04:05 INF event key=value key2=value2
func (l *Logger) writeText(ts, level, event string, fields map[string]interface{}) {
	tag := levelTag[level]

	// Sort field keys for deterministic output.
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	line := fmt.Sprintf("%s %s %s", ts, tag, event)
	for _, k := range keys {
		line += fmt.Sprintf(" %s=%v", k, fields[k])
	}
	line += "\n"

	l.file.WriteString(line)
}
