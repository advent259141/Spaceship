package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	colorReset = "\u001b[0m"
	colorGreen = "\u001b[32m"
)

type Handler struct {
	out    io.Writer
	level  slog.Leveler
	mu     *sync.Mutex
	attrs  []slog.Attr
	groups []string
	tag    string
}

func New(level string) *slog.Logger {
	return slog.New(&Handler{
		out:   os.Stdout,
		level: parseLevel(level),
		mu:    &sync.Mutex{},
		tag:   "[Agent]",
	})
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *Handler) Handle(_ context.Context, record slog.Record) error {
	line := h.formatRecord(record)

	h.mu.Lock()
	defer h.mu.Unlock()

	_, err := fmt.Fprintln(h.out, line)
	return err
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := h.clone()
	clone.attrs = append(clone.attrs, attrs...)
	return clone
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	clone := h.clone()
	clone.groups = append(clone.groups, name)
	return clone
}

func (h *Handler) clone() *Handler {
	attrs := append([]slog.Attr{}, h.attrs...)
	groups := append([]string{}, h.groups...)
	return &Handler{
		out:    h.out,
		level:  h.level,
		mu:     h.mu,
		attrs:  attrs,
		groups: groups,
		tag:    h.tag,
	}
}

func (h *Handler) formatRecord(record slog.Record) string {
	timestamp := record.Time
	if timestamp.IsZero() {
		timestamp = time.Now()
	}

	sourceFile, sourceLine := sourceFromPC(record.PC)
	parts := []string{
		colorGreen + "[" + timestamp.Format("15:04:05.000") + "]" + colorReset,
		h.tag,
		colorForLevel(record.Level) + "[" + shortLevelName(record.Level) + "]" + colorReset,
		fmt.Sprintf("[%s:%d]:", sourceFile, sourceLine),
		record.Message,
	}

	attrText := h.formatAttrs(record)
	if attrText != "" {
		parts = append(parts, attrText)
	}

	return strings.Join(parts, " ")
}

func (h *Handler) formatAttrs(record slog.Record) string {
	attrs := append([]slog.Attr{}, h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		attrs = append(attrs, attr)
		return true
	})
	if len(attrs) == 0 {
		return ""
	}

	parts := make([]string, 0, len(attrs))
	prefix := strings.Join(h.groups, ".")
	for _, attr := range attrs {
		h.appendAttr(&parts, prefix, attr)
	}
	return strings.Join(parts, " ")
}

func (h *Handler) appendAttr(parts *[]string, prefix string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}

	key := attr.Key
	if prefix != "" && key != "" {
		key = prefix + "." + key
	}

	switch attr.Value.Kind() {
	case slog.KindGroup:
		nextPrefix := prefix
		if attr.Key != "" {
			if nextPrefix == "" {
				nextPrefix = attr.Key
			} else {
				nextPrefix += "." + attr.Key
			}
		}
		for _, groupAttr := range attr.Value.Group() {
			h.appendAttr(parts, nextPrefix, groupAttr)
		}
	default:
		if key == "" {
			*parts = append(*parts, formatValue(attr.Value.Any()))
			return
		}
		*parts = append(*parts, key+"="+formatValue(attr.Value.Any()))
	}
}

func sourceFromPC(pc uintptr) (string, int) {
	if pc == 0 {
		return "unknown", 0
	}

	frames := runtime.CallersFrames([]uintptr{pc})
	frame, _ := frames.Next()
	if frame.File == "" {
		return "unknown", frame.Line
	}
	return buildSourceFile(frame.File), frame.Line
}

func buildSourceFile(path string) string {
	dir := filepath.Base(filepath.Dir(path))
	file := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if dir == "." || dir == "" {
		return file
	}
	return dir + "." + file
}

func colorForLevel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "\u001b[31m"
	case level >= slog.LevelWarn:
		return "\u001b[1;33m"
	case level <= slog.LevelDebug:
		return "\u001b[1;34m"
	default:
		return "\u001b[1;36m"
	}
}

func shortLevelName(level slog.Level) string {
	switch {
	case level > slog.LevelError:
		return "CRIT"
	case level >= slog.LevelError:
		return "ERRO"
	case level >= slog.LevelWarn:
		return "WARN"
	case level <= slog.LevelDebug:
		return "DBUG"
	default:
		return "INFO"
	}
}

func formatValue(value any) string {
	switch typed := value.(type) {
	case string:
		return quoteIfNeeded(typed)
	case error:
		return quoteIfNeeded(typed.Error())
	case fmt.Stringer:
		return quoteIfNeeded(typed.String())
	default:
		return fmt.Sprint(typed)
	}
}

func quoteIfNeeded(value string) string {
	if value == "" {
		return "\"\""
	}
	if strings.ContainsAny(value, " \t\r\n=") {
		return strconv.Quote(value)
	}
	return value
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
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
