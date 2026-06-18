// Package log 提供 context-aware slog handler，自动从 context 提取 traceID。
package log

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"github.com/reuben/group-buying/internal/middleware/tracing"
)

// New 创建带有 traceID 自动提取功能的 slog.Logger。
//
//	level: debug | info | warn | error
//	format: text | json
func New(level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: lvl == slog.LevelDebug, // debug 模式显示文件名+行号
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	// 包装成 context-aware handler
	handler = &traceHandler{handler: handler}
	return slog.New(handler)
}

// traceHandler 在日志输出前从 context 提取 traceID。
type traceHandler struct {
	handler slog.Handler
}

func (h *traceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		if tid := tracing.GetTraceID(ctx); tid != "" {
			r.AddAttrs(slog.String("trace_id", tid))
		}
	}
	return h.handler.Handle(ctx, r)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{handler: h.handler.WithGroup(name)}
}
