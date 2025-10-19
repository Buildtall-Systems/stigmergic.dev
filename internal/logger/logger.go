package logger

import (
	"log/slog"
	"os"
	"strings"
)

var Log *slog.Logger

func Init(level string) {
	var slogLevel slog.Level
	switch strings.ToUpper(level) {
	case "DEBUG":
		slogLevel = slog.LevelDebug
	case "INFO":
		slogLevel = slog.LevelInfo
	case "WARN":
		slogLevel = slog.LevelWarn
	case "ERROR":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelError
	}

	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slogLevel,
	})

	Log = slog.New(handler)
	slog.SetDefault(Log)
}

func init() {
	Init("ERROR")
}
