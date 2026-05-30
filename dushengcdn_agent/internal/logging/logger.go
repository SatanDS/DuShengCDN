package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type customTextHandler struct {
	writer io.Writer
	level  slog.Level
	attrs  []slog.Attr
	groups []string
}

func Setup() {
	handler := &customTextHandler{
		writer: os.Stdout,
		level:  parseLevel(os.Getenv("LOG_LEVEL")),
	}
	slog.SetDefault(slog.New(handler))
}

func (h *customTextHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *customTextHandler) Handle(_ context.Context, record slog.Record) error {
	var builder strings.Builder
	builder.WriteString(record.Time.Format("2006-01-02 15:04:05.000"))
	builder.WriteString(" | ")
	builder.WriteString(fmt.Sprintf("%-8s", levelLabel(record.Level)))
	builder.WriteString(" | ")
	builder.WriteString(sourceLocation(record.PC))
	builder.WriteString(" - ")
	builder.WriteString(record.Message)

	attrs := make([]slog.Attr, 0, len(h.attrs)+record.NumAttrs())
	attrs = append(attrs, h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})
	if len(attrs) > 0 {
		builder.WriteString(" | ")
		builder.WriteString(formatAttrs(h.groups, attrs))
	}
	builder.WriteByte('\n')
	_, err := io.WriteString(h.writer, builder.String())
	return err
}

func (h *customTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	cloned := *h
	cloned.attrs = append(slices.Clone(h.attrs), attrs...)
	return &cloned
}

func (h *customTextHandler) WithGroup(name string) slog.Handler {
	if strings.TrimSpace(name) == "" {
		return h
	}
	cloned := *h
	cloned.groups = append(slices.Clone(h.groups), name)
	return &cloned
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func levelLabel(level slog.Level) string {
	switch {
	case level <= slog.LevelDebug:
		return "DEBUG"
	case level < slog.LevelWarn:
		return "INFO"
	case level < slog.LevelError:
		return "WARNING"
	default:
		return "ERROR"
	}
}

func sourceLocation(pc uintptr) string {
	if pc == 0 {
		return "unknown:unknown:0"
	}
	frame, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	fileName := strings.TrimSuffix(filepath.Base(frame.File), filepath.Ext(frame.File))
	if fileName == "" {
		fileName = "unknown"
	}
	functionName := "unknown"
	if frame.Function != "" {
		parts := strings.Split(frame.Function, "/")
		functionName = parts[len(parts)-1]
		if dot := strings.LastIndex(functionName, "."); dot >= 0 && dot < len(functionName)-1 {
			functionName = functionName[dot+1:]
		}
	}
	return fmt.Sprintf("%s:%s:%d", fileName, functionName, frame.Line)
}

func formatAttrs(groups []string, attrs []slog.Attr) string {
	parts := make([]string, 0, len(attrs))
	for _, attr := range attrs {
		key := attr.Key
		if key == "" {
			continue
		}
		if len(groups) > 0 {
			key = strings.Join(append(slices.Clone(groups), key), ".")
		}
		parts = append(parts, fmt.Sprintf("%s=%v", key, attr.Value.Any()))
	}
	return strings.Join(parts, " ")
}
