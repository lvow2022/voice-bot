// Package logger provides structured logging using slog.
package logger

import (
	"log/slog"
	"os"
)

var defaultLogger *slog.Logger

func init() {
	// Default to JSON handler for production
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}
	if os.Getenv("DEBUG") != "" {
		opts.Level = slog.LevelDebug
	}

	h := slog.NewJSONHandler(os.Stdout, opts)
	defaultLogger = slog.New(h)
	slog.SetDefault(defaultLogger)
}

// SetDefault sets the default logger.
func SetDefault(l *slog.Logger) {
	defaultLogger = l
	slog.SetDefault(l)
}

// Debug logs a debug message.
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

// DebugCF logs a debug message with component and fields.
func DebugCF(component string, msg string, fields map[string]any) {
	args := []any{"component", component}
	for k, v := range fields {
		args = append(args, k, v)
	}
	defaultLogger.Debug(msg, args...)
}

// Info logs an info message.
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// InfoCF logs an info message with component and fields.
func InfoCF(component string, msg string, fields map[string]any) {
	args := []any{"component", component}
	for k, v := range fields {
		args = append(args, k, v)
	}
	defaultLogger.Info(msg, args...)
}

// Warn logs a warning message.
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// WarnCF logs a warning message with component and fields.
func WarnCF(component string, msg string, fields map[string]any) {
	args := []any{"component", component}
	for k, v := range fields {
		args = append(args, k, v)
	}
	defaultLogger.Warn(msg, args...)
}

// Error logs an error message.
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

// ErrorCF logs an error message with component and fields.
func ErrorCF(component string, msg string, fields map[string]any) {
	args := []any{"component", component}
	for k, v := range fields {
		args = append(args, k, v)
	}
	defaultLogger.Error(msg, args...)
}

// With returns a logger with additional context.
func With(args ...any) *slog.Logger {
	return defaultLogger.With(args...)
}

// WithComponent returns a logger with a component field.
func WithComponent(component string) *slog.Logger {
	return defaultLogger.With("component", component)
}
