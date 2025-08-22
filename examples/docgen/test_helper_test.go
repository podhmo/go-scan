package main

import (
	"flag"
	"io"
	"log/slog"
)

var (
	update = flag.Bool("update", false, "update golden files")
	debug  = flag.Bool("debug", false, "enable debug logging")
)

func newTestLogger(w io.Writer) *slog.Logger {
	level := slog.LevelWarn
	if *debug {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}
