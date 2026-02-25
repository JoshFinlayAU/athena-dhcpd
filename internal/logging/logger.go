// Package logging provides slog setup helpers for athena-dhcpd.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// Setup initializes the default slog logger with the given level and output.
func Setup(level string, output io.Writer) *slog.Logger {
	if output == nil {
		output = os.Stdout
	}

	var lvl slog.Level
	switch strings.ToLower(level) {
	case "trace", "debug":
		lvl = slog.LevelDebug
	case "info", "":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: lvl,
	}

	handler := slog.NewJSONHandler(output, opts)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// ParseLevel converts a string level to slog.Level.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "trace", "debug":
		return slog.LevelDebug
	case "info", "":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
