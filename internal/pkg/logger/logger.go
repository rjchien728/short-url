package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

// ctxKey is a custom type for context keys to avoid collisions.
type ctxKey string

// loggerKey is the context key used to store logger instances.
const loggerKey ctxKey = "logger"

// Setup configures the global slog.Default() logger with the specified level and format.
// Supported levels: "debug", "info", "warn", "error" (case-insensitive).
// Format: "json" for JSON output, anything else for text output.
func Setup(level, format string) error {
	logLevel, err := parseLevel(level)
	if err != nil {
		return err
	}

	opts := &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: true,
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return nil
}

func parseLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level: %s", level)
	}
}

// WithLogger returns a new context with the given logger attached.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

func fromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// FromContext retrieves the logger from the context, or returns the default logger.
// This is useful for extracting loggers with attached metadata (like requestID)
// and attaching them to new contexts.
func FromContext(ctx context.Context) *slog.Logger {
	return fromContext(ctx)
}

// With returns a new context with a logger that has the given attributes.
func With(ctx context.Context, args ...any) context.Context {
	l := fromContext(ctx).With(args...)
	return WithLogger(ctx, l)
}

// log is an internal helper that captures the correct caller location.
// It uses runtime.Callers to skip 3 frames [Callers, log, Info/Debug/Error/Warn]
// so that AddSource points to the actual caller, not this wrapper.
func log(ctx context.Context, level slog.Level, msg string, args ...any) {
	logger := fromContext(ctx)
	if !logger.Enabled(ctx, level) {
		return
	}

	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])

	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = logger.Handler().Handle(ctx, r)
}

// Debug logs a message at Debug level.
func Debug(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.LevelDebug, msg, args...)
}

// Info logs a message at Info level.
func Info(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.LevelInfo, msg, args...)
}

// Warn logs a message at Warn level.
func Warn(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.LevelWarn, msg, args...)
}

// Error logs a message at Error level.
func Error(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.LevelError, msg, args...)
}
