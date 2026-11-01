// Package config reads this machine's settings: ~/.skl/config.toml, the
// SKL_* environment variables and the defaults (docs/reference/config.md),
// and computes the paths of skl's local files (paths.go).
//
// A flag wins over an environment variable, which wins over the file, which
// wins over the default. Only the value that wins is checked. The caller
// parses flags, such as --scope with ParseScope, and passes them to Load.
package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Protocol is how skl reaches repos written as owner/repo or host/path.
type Protocol string

// Protocols.
const (
	HTTPS Protocol = "https"
	SSH   Protocol = "ssh"
)

// Scope is where skills are loaded: for the user, or for the current repo.
type Scope string

// Scopes.
const (
	User    Scope = "user"
	Project Scope = "project"
)

// Target is a family of skill folders skl loads into.
type Target string

// Targets: claude is ~/.claude/skills, agents is ~/.agents/skills
// (.claude/skills and .agents/skills in project scope).
const (
	Claude Target = "claude"
	Agents Target = "agents"
)

// Config is this machine's settings, resolved.
type Config struct {
	DefaultHost string   // the host owner/repo refers to
	Protocol    Protocol // for owner/repo and host/path repos
	Scope       Scope    // the default for --scope
	Targets     []Target // in order, without repeats, never empty
	Engine      string   // "claude", or a command template for lessons
	License     string   // an SPDX id or "none", for skl init's LICENSE
}

// Default returns the settings skl uses without a file or variables.
func Default() Config {
	return Config{
		DefaultHost: "github.com",
		Protocol:    HTTPS,
		Scope:       User,
		Targets:     []Target{Claude},
		Engine:      "claude",
		License:     "MIT",
	}
}

// InvalidError is a config file that doesn't parse, or a setting with a
// value skl can't use.
type InvalidError struct {
	Key    string // the setting, such as "protocol"; empty if the file doesn't parse
	Var    string // the environment variable it came from; empty if from the file
	File   string // the config file, with ~ for HOME
	Reason string // what's wrong, starting with the key or the variable
}

func (e *InvalidError) Error() string { return "config: " + e.Reason }

// Hint is the command that deals with the error: edit the file, or unset
// the variable that holds the bad value.
func (e *InvalidError) Hint() string {
	switch {
	case e.Var != "":
		return "unset " + e.Var
	case e.Key != "":
		return "edit " + e.Key + " in " + e.File
	default:
		return "edit " + e.File
	}
}

// UnreadableError is a config file that exists but can't be read.
type UnreadableError struct {
	File string // the config file, with ~ for HOME
	Err  error  // why, such as permission denied
}

func (e *UnreadableError) Error() string { return "can't read " + e.File + ": " + e.Err.Error() }
func (e *UnreadableError) Unwrap() error { return e.Err }

// Hint is what deals with the error.
func (e *UnreadableError) Hint() string { return "check its permissions" }

// Flags are the command-line flags that override a setting. A zero field is
// a flag that wasn't given.
type Flags struct {
	Scope Scope // --scope, parsed with ParseScope
}

// Load reads p.Config, if it exists, the SKL_* variables over it and the
// flags over those. A missing file is fine; a file skl can't read is an
// *UnreadableError, and a bad value an *InvalidError.
func Load(ctx context.Context, p Paths, getenv func(string) string, flags Flags) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	shown := Tilde(p.Config, getenv("HOME"))
	var inFile map[string]any
	data, err := os.ReadFile(p.Config)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Config{}, &UnreadableError{File: shown, Err: osReason(err)}
	default:
		if inFile, err = parseFile(data, shown); err != nil {
			return Config{}, err
		}
	}

	c := Default()
	if flags.Scope != "" {
		c.Scope = flags.Scope
	}
	settings := []struct {
		key, envVar string
		// apply sets v on c, or says what's wrong with it; name is the key
		// or the variable v came from.
		apply func(name, v string) string
		// applyFile is apply for the file's value, when that isn't a string.
		applyFile func(v any) string
		// flagged says a flag overrides the setting.
		flagged bool
	}{
		{key: "default_host", envVar: "SKL_DEFAULT_HOST", apply: func(name, v string) string {
			if v == "" || strings.ContainsAny(v, "/ \t") {
				return fmt.Sprintf("%s must be a host name such as github.com, not %q", name, v)
			}
			c.DefaultHost = v
			return ""
		}},
		{key: "protocol", envVar: "SKL_PROTOCOL", apply: func(name, v string) string {
			if v != string(HTTPS) && v != string(SSH) {
				return fmt.Sprintf("%s must be ssh or https, not %q", name, v)
			}
			c.Protocol = Protocol(v)
			return ""
		}},
		{key: "scope", envVar: "SKL_SCOPE", apply: func(name, v string) string {
			s, err := parseScope(name, v)
			if err != nil {
				return err.Error()
			}
			c.Scope = s
			return ""
		}, flagged: flags.Scope != ""},
		{key: "targets", envVar: "SKL_TARGETS", apply: func(name, v string) string {
			// Comma-separated; empty items are skipped.
			var names []string
			for w := range strings.SplitSeq(v, ",") {
				if w = strings.TrimSpace(w); w != "" {
					names = append(names, w)
				}
			}
			ts, reason := parseTargets(name, names)
			if reason == "" {
				c.Targets = ts
			}
			return reason
		}, applyFile: func(v any) string {
			names, ok := stringList(v)
			if !ok {
				return "targets must be a list of target names"
			}
			ts, reason := parseTargets("targets", names)
			if reason == "" {
				c.Targets = ts
			}
			return reason
		}},
		{key: "engine", envVar: "SKL_ENGINE", apply: func(name, v string) string {
			if strings.TrimSpace(v) == "" {
				return name + " is empty"
			}
			c.Engine = v
			return ""
		}},
		{key: "license", envVar: "SKL_LICENSE", apply: func(name, v string) string {
			if v != "none" && (!spdxID.MatchString(v) || notALicense(v)) {
				return fmt.Sprintf("%s must be an SPDX id such as MIT, or none, not %q", name, v)
			}
			c.License = v
			return ""
		}},
	}
	for _, s := range settings {
		if s.flagged {
			continue
		}
		// An empty variable counts as unset.
		if v := getenv(s.envVar); v != "" {
			if why := s.apply(s.envVar, v); why != "" {
				return Config{}, &InvalidError{Key: s.key, Var: s.envVar, File: shown, Reason: why}
			}
			continue
		}
		v, ok := inFile[s.key]
		if !ok {
			continue
		}
		var why string
		switch str, isStr := v.(string); {
		case s.applyFile != nil:
			why = s.applyFile(v)
		case isStr:
			why = s.apply(s.key, str)
		default:
			why = s.key + " must be a string in quotes, not " + tomlValue(v)
		}
		if why != "" {
			return Config{}, &InvalidError{Key: s.key, File: shown, Reason: why}
		}
	}
	return c, nil
}

// osReason is err without the operation and path os puts around it.
func osReason(err error) error {
	if pe, ok := errors.AsType[*fs.PathError](err); ok {
		return pe.Err
	}
	return err
}

// parseTargets checks a list of target names and returns it in order,
// without repeats. The list can't be empty.
func parseTargets(name string, names []string) ([]Target, string) {
	var ts []Target
	for _, n := range names {
		t := Target(n)
		switch {
		case t != Claude && t != Agents:
			return nil, fmt.Sprintf("%s can only hold claude and agents, not %q", name, n)
		case !slices.Contains(ts, t):
			ts = append(ts, t)
		}
	}
	if len(ts) == 0 {
		return nil, name + " is empty: name claude, agents or both"
	}
	return ts, ""
}

// stringList is v as a list of strings, if it is one.
func stringList(v any) ([]string, bool) {
	list, ok := v.([]any)
	if !ok {
		return nil, false
	}
	names := make([]string, len(list))
	for i, e := range list {
		if names[i], ok = e.(string); !ok {
			return nil, false
		}
	}
	return names, true
}

// spdxID is the shape of an SPDX license id, such as Apache-2.0 or
// GPL-3.0-or-later; LGPL-2.1+ is an older form still in use. Whether skl
// knows the license's text is a question for skl init.
var spdxID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*\+?$`)

// notALicense reports the words that have an SPDX id's shape but say there
// is no license. "none", in lower case, is the setting for that.
func notALicense(v string) bool {
	return strings.EqualFold(v, "none") || strings.EqualFold(v, "noassertion")
}

// ParseScope checks a --scope value.
func ParseScope(v string) (Scope, error) { return parseScope("scope", v) }

func parseScope(name, v string) (Scope, error) {
	if s := Scope(v); s == User || s == Project {
		return s, nil
	}
	return "", fmt.Errorf("%s must be user or project, not %q", name, v)
}

// parseFile decodes config.toml. Load picks the keys core owns; other keys
// and tables, such as [web] or a third-party part's table, are left alone:
// core ignores what it doesn't own.
func parseFile(data []byte, shown string) (map[string]any, error) {
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		reason := shown + ": " + err.Error()
		if pe, ok := errors.AsType[toml.ParseError](err); ok {
			reason = fmt.Sprintf("%s line %d: %s", shown, pe.Position.Line, pe.Message)
		}
		return nil, &InvalidError{File: shown, Reason: reason}
	}
	return raw, nil
}

// tomlValue shows a decoded value roughly as it was written.
func tomlValue(v any) string {
	switch v := v.(type) {
	case string:
		return strconv.Quote(v)
	case []any:
		parts := make([]string, len(v))
		for i, e := range v {
			parts[i] = tomlValue(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any, []map[string]any:
		return "a table"
	default:
		return fmt.Sprint(v)
	}
}

// Tilde shows path with ~ for home, as the docs write it.
func Tilde(path, home string) string {
	if home == "" {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join("~", rel)
}
