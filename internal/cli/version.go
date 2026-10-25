package cli

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// version and commit are set at release time with
// -ldflags "-X github.com/farzeni/skl/internal/cli.version=… -X …commit=…".
// Left empty, they come from the build info, so `go install …@latest`
// prints a real version.
var (
	version string
	commit  string
)

// pseudoVersion matches the versions Go makes up for an untagged commit,
// such as v0.0.0-20261025120000-b2618b8abcde.
var pseudoVersion = regexp.MustCompile(`-(0\.)?\d{14}-[0-9a-f]{12}`)

// versionLine is what skl --version prints: "skl 0.1.0 (b0a0fb2)".
func versionLine() string {
	v, c := version, commit
	if info, ok := debug.ReadBuildInfo(); ok {
		if v == "" {
			v = info.Main.Version
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && c == "" {
				c = s.Value
			}
		}
	}
	v = strings.TrimSuffix(strings.TrimPrefix(v, "v"), "+dirty")
	if v == "" || v == "(devel)" || pseudoVersion.MatchString(v) {
		v = "dev"
	}
	line := "skl " + v
	if c != "" {
		line += " (" + c[:min(7, len(c))] + ")"
	}
	return line
}
