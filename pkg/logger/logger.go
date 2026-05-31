package logger

import (
	"log/slog"
	"os"
)

// New creates a structured slog logger.
// In production (env != "development") it uses JSON format; otherwise text format.
func New(env string, level slog.Level) *slog.Logger {
	var handler slog.Handler

	opts := &slog.HandlerOptions{
		Level: level,
	}

	if env == "development" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
