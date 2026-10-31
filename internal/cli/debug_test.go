package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/farzeni/skl/internal/gitx"
)

// docs/reference/config.md: "SKL_DEBUG: Set to 1 to print each git and
// engine command skl runs, on standard error."
func TestDebugLogger(t *testing.T) {
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	dir := t.TempDir()
	for _, tt := range []struct{ debug, want string }{
		{"1", "level=DEBUG msg=\"git version\"\n" +
			"level=DEBUG msg=\"git config --get core.sshCommand\" dir=" + dir + "\n" +
			"level=DEBUG msg=\"git init --quiet\" dir=" + dir + "\n"},
		{"", ""},
		{"0", ""},
		{"true", ""},
	} {
		var stderr bytes.Buffer
		getenv := func(k string) string {
			if k == "SKL_DEBUG" {
				return tt.debug
			}
			return ""
		}
		g := gitx.New(debugLogger(&stderr, getenv))
		if _, err := g.Run(t.Context(), dir, "init", "--quiet"); err != nil {
			t.Fatal(err)
		}
		if got := stderr.String(); got != tt.want {
			t.Errorf("SKL_DEBUG=%q: stderr = %q, want %q", tt.debug, got, tt.want)
		}
		if strings.Contains(stderr.String(), "time=") {
			t.Errorf("debug lines carry the time: %q", stderr.String())
		}
	}
}
