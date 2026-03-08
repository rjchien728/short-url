package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		desc        string
		level       string
		format      string
		expectedErr bool
	}{
		{
			desc:        "valid debug level text format",
			level:       "debug",
			format:      "text",
			expectedErr: false,
		},
		{
			desc:        "valid info level json format",
			level:       "info",
			format:      "json",
			expectedErr: false,
		},
		{
			desc:        "valid warn level",
			level:       "warn",
			format:      "text",
			expectedErr: false,
		},
		{
			desc:        "valid error level case-insensitive",
			level:       "ERROR",
			format:      "text",
			expectedErr: false,
		},
		{
			desc:        "unknown level returns error",
			level:       "verbose",
			format:      "text",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			err := Setup(tt.level, tt.format)
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWithLogger_FromContext(t *testing.T) {
	t.Run("returns attached logger from context", func(t *testing.T) {
		buf := &bytes.Buffer{}
		customLogger := slog.New(slog.NewTextHandler(buf, nil))

		ctx := context.Background()
		ctx = WithLogger(ctx, customLogger)

		got := FromContext(ctx)
		require.NotNil(t, got)

		// 寫入 log 後確認是寫到 buf（同一個 handler）
		got.Info("test-message")
		assert.Contains(t, buf.String(), "test-message")
	})

	t.Run("returns default logger when context has no logger", func(t *testing.T) {
		ctx := context.Background()
		got := FromContext(ctx)
		require.NotNil(t, got)
		// 沒有 panic 即可，回傳 slog.Default()
		assert.Equal(t, slog.Default(), got)
	})

	t.Run("WithLogger stores different loggers independently", func(t *testing.T) {
		buf1 := &bytes.Buffer{}
		buf2 := &bytes.Buffer{}
		logger1 := slog.New(slog.NewTextHandler(buf1, nil))
		logger2 := slog.New(slog.NewTextHandler(buf2, nil))

		ctx1 := WithLogger(context.Background(), logger1)
		ctx2 := WithLogger(context.Background(), logger2)

		FromContext(ctx1).Info("msg-for-1")
		FromContext(ctx2).Info("msg-for-2")

		assert.Contains(t, buf1.String(), "msg-for-1")
		assert.NotContains(t, buf1.String(), "msg-for-2")
		assert.Contains(t, buf2.String(), "msg-for-2")
		assert.NotContains(t, buf2.String(), "msg-for-1")
	})
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		desc          string
		input         string
		expectedLevel slog.Level
		expectedErr   bool
	}{
		{"debug lowercase", "debug", slog.LevelDebug, false},
		{"info lowercase", "info", slog.LevelInfo, false},
		{"warn lowercase", "warn", slog.LevelWarn, false},
		{"error lowercase", "error", slog.LevelError, false},
		{"mixed case WARN", "WARN", slog.LevelWarn, false},
		{"unknown level", "trace", slog.LevelInfo, true},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			lvl, err := parseLevel(tt.input)
			if tt.expectedErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "unknown log level")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedLevel, lvl)
			}
		})
	}
}

func TestLogFunctions_DoNotPanic(t *testing.T) {
	// 確認四個 log 函數在各種情況下不 panic
	buf := &bytes.Buffer{}
	l := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := WithLogger(context.Background(), l)

	assert.NotPanics(t, func() { Debug(ctx, "debug msg", "key", "val") })
	assert.NotPanics(t, func() { Info(ctx, "info msg") })
	assert.NotPanics(t, func() { Warn(ctx, "warn msg") })
	assert.NotPanics(t, func() { Error(ctx, "error msg") })

	output := buf.String()
	assert.True(t, strings.Contains(output, "debug msg"))
	assert.True(t, strings.Contains(output, "info msg"))
	assert.True(t, strings.Contains(output, "warn msg"))
	assert.True(t, strings.Contains(output, "error msg"))
}
