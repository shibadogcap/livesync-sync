// Package logger provides structured logging for livesync-sync.
// Uses Go's log/slog package with level control and optional file output.
package logger

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Init sets up the global logger based on the provided level and optional file path.
// Level values: "debug", "info", "warn", "error"
func Init(level, filePath string) error {
	var levelVar slog.Level

	switch strings.ToLower(level) {
	case "debug":
		levelVar = slog.LevelDebug
	case "info":
		levelVar = slog.LevelInfo
	case "warn":
		levelVar = slog.LevelWarn
	case "error":
		levelVar = slog.LevelError
	default:
		levelVar = slog.LevelInfo
	}

	var writer io.Writer = os.Stderr

	if filePath != "" {
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}
		writer = f
		// Note: log rotation is not implemented yet.
		// For production use, consider using https://pkg.go.dev/gopkg.in/natefinch/lumberjack.v2
	}

	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: levelVar,
	})

	slog.SetDefault(slog.New(handler))
	return nil
}
