// Package logging sends info and debug records to one writer and warnings
// and errors to another. Under procd, stdout is logged at info priority and
// stderr at err priority.
package logging

import (
	"context"
	"io"
	"log/slog"
)

type splitHandler struct {
	info slog.Handler
	err  slog.Handler
}

// New returns a logger that writes records below slog.LevelWarn to info and
// the rest to errOut.
func New(info, errOut io.Writer) *slog.Logger {
	opts := &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}
	return slog.New(&splitHandler{
		info: slog.NewTextHandler(info, opts),
		err:  slog.NewTextHandler(errOut, opts),
	})
}

func (h *splitHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.info.Enabled(ctx, level)
}

func (h *splitHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		return h.err.Handle(ctx, r)
	}
	return h.info.Handle(ctx, r)
}

func (h *splitHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &splitHandler{info: h.info.WithAttrs(attrs), err: h.err.WithAttrs(attrs)}
}

func (h *splitHandler) WithGroup(name string) slog.Handler {
	return &splitHandler{info: h.info.WithGroup(name), err: h.err.WithGroup(name)}
}
