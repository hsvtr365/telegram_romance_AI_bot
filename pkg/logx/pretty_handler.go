package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type PrettyHandler struct {
	out    io.Writer
	opts   slog.HandlerOptions
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

func NewPrettyHandler(out io.Writer, opts *slog.HandlerOptions) *PrettyHandler {
	var copied slog.HandlerOptions
	if opts != nil {
		copied = *opts
	}
	return &PrettyHandler{
		out:  out,
		opts: copied,
		mu:   &sync.Mutex{},
	}
}

func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *PrettyHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("[%s] %s", strings.ToUpper(r.Level.String()), r.Time.Format(time.DateTime)))
	if r.Message != "" {
		b.WriteString(" ")
		b.WriteString(r.Message)
	}
	b.WriteString("\n")

	for _, attr := range h.attrs {
		h.writeAttr(&b, attr)
	}

	r.Attrs(func(attr slog.Attr) bool {
		h.writeAttr(&b, attr)
		return true
	})

	b.WriteString("\n")

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.out, b.String())
	return err
}

func (h *PrettyHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &PrettyHandler{
		out:    h.out,
		opts:   h.opts,
		attrs:  merged,
		groups: append([]string(nil), h.groups...),
		mu:     h.mu,
	}
}

func (h *PrettyHandler) WithGroup(name string) slog.Handler {
	if strings.TrimSpace(name) == "" {
		return h
	}
	nextGroups := append([]string(nil), h.groups...)
	nextGroups = append(nextGroups, name)
	return &PrettyHandler{
		out:    h.out,
		opts:   h.opts,
		attrs:  append([]slog.Attr(nil), h.attrs...),
		groups: nextGroups,
		mu:     h.mu,
	}
}

func (h *PrettyHandler) writeAttr(b *strings.Builder, attr slog.Attr) {
	if attr.Equal(slog.Attr{}) {
		return
	}

	attr.Value = attr.Value.Resolve()
	key := attr.Key
	if len(h.groups) > 0 {
		key = strings.Join(append(append([]string(nil), h.groups...), key), ".")
	}

	b.WriteString("  - ")
	b.WriteString(key)
	b.WriteString(": ")
	b.WriteString(formatValue(attr.Value))
	b.WriteString("\n")
}

func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindInt64:
		return fmt.Sprintf("%d", v.Int64())
	case slog.KindUint64:
		return fmt.Sprintf("%d", v.Uint64())
	case slog.KindFloat64:
		return fmt.Sprintf("%g", v.Float64())
	case slog.KindBool:
		return fmt.Sprintf("%t", v.Bool())
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindGroup:
		parts := make([]string, 0, len(v.Group()))
		for _, child := range v.Group() {
			child.Value = child.Value.Resolve()
			parts = append(parts, child.Key+"="+formatValue(child.Value))
		}
		return strings.Join(parts, ", ")
	case slog.KindAny:
		return fmt.Sprintf("%v", v.Any())
	default:
		return v.String()
	}
}
