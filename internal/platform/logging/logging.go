// Package logging creates structured process loggers.
package logging

import (
	"io"
	"log/slog"
)

// New writes JSON log records to w.
func New(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
