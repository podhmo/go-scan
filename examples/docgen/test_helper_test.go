package main

import (
	"io"
	"log/slog"
)

func newTestLogger(w io.Writer) *slog.Logger {
	level := slog.LevelWarn
	if *debug {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}
