// Package logger is the process-wide structured logger. Call sites pass
// a context and keyed fields. Debug lines are emitted only when
// APP_ENV=development.
package logger

import (
	"context"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Field is one structured value on a log line.
type Field struct {
	Key string
	Val any
}

// Logger writes keyed lines. Implementations must not log secrets.
type Logger interface {
	Debug(ctx context.Context, message string, fields ...Field)
	Info(ctx context.Context, message string, fields ...Field)
	Warn(ctx context.Context, message string, fields ...Field)
	Error(ctx context.Context, message string, fields ...Field)
}

var (
	instance Logger
	once     sync.Once
)

// Get returns the process logger, creating it on first use.
func Get() Logger {
	once.Do(func() {
		log, err := newZap()
		if err != nil {
			panic(err)
		}
		instance = &zapLogger{log: log}
	})
	return instance
}

// ToField builds a field without a positional composite at the call site.
func ToField(key string, val any) Field {
	return Field{Key: key, Val: val}
}

func newZap() (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if os.Getenv("APP_ENV") == "development" {
		level = zapcore.DebugLevel
	}
	cfg := zap.NewProductionConfig()
	cfg.Level = zap.NewAtomicLevelAt(level)
	return cfg.Build()
}

type zapLogger struct {
	log *zap.Logger
}

func (z *zapLogger) Debug(ctx context.Context, message string, fields ...Field) {
	z.log.Debug(message, toZap(ctx, fields)...)
}

func (z *zapLogger) Info(ctx context.Context, message string, fields ...Field) {
	z.log.Info(message, toZap(ctx, fields)...)
}

func (z *zapLogger) Warn(ctx context.Context, message string, fields ...Field) {
	z.log.Warn(message, toZap(ctx, fields)...)
}

func (z *zapLogger) Error(ctx context.Context, message string, fields ...Field) {
	z.log.Error(message, toZap(ctx, fields)...)
}

func toZap(ctx context.Context, fields []Field) []zap.Field {
	out := make([]zap.Field, 0, len(fields)+1)
	if id := RequestID(ctx); id != "" {
		out = append(out, zap.String("request_id", id))
	}
	for _, field := range fields {
		out = append(out, zap.Any(field.Key, field.Val))
	}
	return out
}
