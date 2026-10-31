// Package config reads this machine's settings: ~/.skl/config.toml, the
// SKL_* environment variables and the defaults (docs/reference/config.md),
// and computes the paths of skl's local files (paths.go).
//
// A flag wins over an environment variable, which wins over the file, which
// wins over the default. Load handles the last three; the caller applies
// flags, such as --scope with ParseScope.
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

// Load reads p.Config, if it exists, and the SKL_* variables over it.
// A missing file is fine; a bad value is an *InvalidError.
func Load(ctx context.Context, p Paths, getenv func(string) string) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}
	shown := tilde(p.Config, getenv("HOME"))
	var inFile map[string]string
	data, err := os.ReadFile(p.Config)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return Config{}, fmt.Errorf("config: %w", err)
	default:
		if inFile, err = parseFile(data, shown); err != nil {
			return Config{}, err
		}
	}

	c := Default()
	settings := []struct {
		key, envVar string
		// apply sets v on c, or says what's wrong with it; name is the key
		// or the variable v came from.
		apply func(name, v string) string
	}{
		{"default_host", "SKL_DEFAULT_HOST", func(name, v string) string {
			if v == "" || strings.ContainsAny(v, "/ \t") {
				return fmt.Sprintf("%s must be a host name such as github.com, not %q", name, v)
			}
			c.DefaultHost = v
			return ""
		}},
		{"protocol", "SKL_PROTOCOL", func(name, v string) string {
			if v != string(HTTPS) && v != string(SSH) {
				return fmt.Sprintf("%s must be ssh or https, not %q", name, v)
			}
			c.Protocol = Protocol(v)
			return ""
		}},
		{"scope", "SKL_SCOPE", func(name, v string) string {
			s, err := parseScope(name, v)
			if err != nil {
				return err.Error()
			}
			c.Scope = s
			return ""
		}},
		{"targets", "SKL_TARGETS", func(name, v string) string {
			ts, reason := parseTargets(name, v)
			if reason == "" {
				c.Targets = ts
			}
			return reason
		}},
		{"engine", "SKL_ENGINE", func(name, v string) string {
			if strings.TrimSpace(v) == "" {
				return name + " is empty"
			}
			c.Engine = v
			return ""
		}},
		{"license", "SKL_LICENSE", func(name, v string) string {
			if v != "none" && !spdxID.MatchString(v) {
				return fmt.Sprintf("%s must be an SPDX id such as MIT, or none, not %q", name, v)
			}
			c.License = v
			return ""
		}},
	}
	for _, s := range settings {
		// An empty variable counts as unset.
		name, v := s.envVar, getenv(s.envVar)
		if v == "" {
			fv, ok := inFile[s.key]
			if !ok {
				continue
			}
			name, v = s.key, fv
		}
		if reason := s.apply(name, v); reason != "" {
			ie := &InvalidError{Key: s.key, File: shown, Reason: reason}
			if name == s.envVar {
				ie.Var = s.envVar
			}
			return Config{}, ie
		}
	}
	return c, nil
}

// parseTargets reads a comma-separated list of targets, in order, without
// repeats. Empty items are skipped, but the list can't be empty.
func parseTargets(name, v string) ([]Target, string) {
	var ts []Target
	for w := range strings.SplitSeq(v, ",") {
		t := Target(strings.TrimSpace(w))
		switch {
		case t == "":
		case t != Claude && t != Agents:
			return nil, fmt.Sprintf("%s can only hold claude and agents, not %q", name, t)
		case !slices.Contains(ts, t):
			ts = append(ts, t)
		}
	}
	if len(ts) == 0 {
		return nil, name + " is empty: name claude, agents or both"
	}
	return ts, ""
}

// spdxID is the shape of an SPDX license id, such as Apache-2.0 or
// GPL-3.0-or-later; LGPL-2.1+ is an older form still in use. Whether skl
// knows the license's text is a question for skl init.
var spdxID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*\+?$`)

// ParseScope checks a --scope value.
func ParseScope(v string) (Scope, error) { return parseScope("scope", v) }

func parseScope(name, v string) (Scope, error) {
	if s := Scope(v); s == User || s == Project {
		return s, nil
	}
	return "", fmt.Errorf("%s must be user or project, not %q", name, v)
}

// parseFile reads the keys core owns from config.toml, as strings; targets
// becomes a comma-separated list, like SKL_TARGETS. Other keys and tables,
// such as [web] or a third-party part's table, are left alone: core ignores
// what it doesn't own.
func parseFile(data []byte, shown string) (map[string]string, error) {
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		reason := shown + ": " + err.Error()
		if pe, ok := errors.AsType[toml.ParseError](err); ok {
			reason = fmt.Sprintf("%s line %d: %s", shown, pe.Position.Line, pe.Message)
		}
		return nil, &InvalidError{File: shown, Reason: reason}
	}
	out := map[string]string{}
	for _, key := range []string{"default_host", "protocol", "scope", "engine", "license"} {
		v, ok := raw[key]
		if !ok {
			continue
		}
		s, ok := v.(string)
		if !ok {
			return nil, &InvalidError{Key: key, File: shown, Reason: key + " must be a string in quotes, not " + tomlValue(v)}
		}
		out[key] = s
	}
	if v, ok := raw["targets"]; ok {
		list, isList := v.([]any)
		names := make([]string, 0, len(list))
		for _, t := range list {
			if s, ok := t.(string); ok {
				names = append(names, s)
			}
		}
		if !isList || len(names) != len(list) {
			return nil, &InvalidError{Key: "targets", File: shown, Reason: `targets must be a list such as ["claude", "agents"], not ` + tomlValue(v)}
		}
		out["targets"] = strings.Join(names, ",")
	}
	return out, nil
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
	default:
		return fmt.Sprint(v)
	}
}

// tilde shows path with ~ for home, as the docs write it.
func tilde(path, home string) string {
	if home == "" {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join("~", rel)
}
