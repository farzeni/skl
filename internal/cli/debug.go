package cli

import (
	"io"
	"log/slog"
)

// debugLogger logs to stderr when SKL_DEBUG=1 and nowhere otherwise:
// "SKL_DEBUG: Set to 1 to print each git and engine command skl runs, on
// standard error" (docs/reference/config.md). The lines carry no time, so
// two runs compare.
func debugLogger(stderr io.Writer, getenv func(string) string) *slog.Logger {
	if getenv("SKL_DEBUG") != "1" {
		return slog.New(slog.DiscardHandler)
	}
	return slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
}
