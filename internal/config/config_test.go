package config

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/farzeni/skl/internal/ui"
)

// env returns a getenv over a fixed map.
func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

// load writes body to a config file in a fresh SKL_HOME (none if body is
// "-") and loads it with the given environment on top.
func load(t *testing.T, body string, vars map[string]string) (Config, error) {
	t.Helper()
	home := t.TempDir()
	p, err := NewPaths(env(map[string]string{"HOME": home}))
	if err != nil {
		t.Fatal(err)
	}
	if body != "-" {
		if err := os.MkdirAll(p.Home, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p.Config, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	all := map[string]string{"HOME": home}
	maps.Copy(all, vars)
	return Load(context.Background(), p, env(all), Flags{})
}

// config.md: "without it, skl uses the defaults below".
func TestDefaults(t *testing.T) {
	want := Config{
		DefaultHost: "github.com",
		Protocol:    HTTPS,
		Scope:       User,
		Targets:     []Target{Claude},
		Engine:      "claude",
		License:     "MIT",
	}
	if got := Default(); !reflect.DeepEqual(got, want) {
		t.Errorf("Default() = %+v, want %+v", got, want)
	}
	// "Create and edit it by hand when you want to change a default": the
	// file is optional.
	got, err := load(t, "-", nil)
	if err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("missing file = %+v, want %+v", got, want)
	}
	got, err = load(t, "", nil)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("empty file = %+v, %v; want %+v", got, err, want)
	}
}

// config.md "Precedence": "A command-line flag wins over an environment
// variable, which wins over the config file, which wins over the default."
// And "Environment variables": "Every key above the [web] table can be set
// with an environment variable".
func TestPrecedence(t *testing.T) {
	keys := []struct {
		key, envVar   string
		file, fromEnv string // TOML value, env value
		def, f, e     any    // default, from the file, from the env
		get           func(Config) any
	}{
		{
			"default_host", "SKL_DEFAULT_HOST", `"gitlab.example.com"`, "git.example.org",
			"github.com", "gitlab.example.com", "git.example.org", func(c Config) any { return c.DefaultHost },
		},
		{
			"protocol", "SKL_PROTOCOL", `"ssh"`, "https",
			HTTPS, SSH, HTTPS, func(c Config) any { return c.Protocol },
		},
		{
			"scope", "SKL_SCOPE", `"project"`, "user",
			User, Project, User, func(c Config) any { return c.Scope },
		},
		{
			"targets", "SKL_TARGETS", `["agents"]`, "claude,agents",
			[]Target{Claude},
			[]Target{Agents},
			[]Target{Claude, Agents},
			func(c Config) any { return c.Targets },
		},
		{
			"engine", "SKL_ENGINE", `"pi --skill {skill} {prompt}"`, "opencode --prompt {prompt}",
			"claude", "pi --skill {skill} {prompt}", "opencode --prompt {prompt}", func(c Config) any { return c.Engine },
		},
		{
			"license", "SKL_LICENSE", `"Apache-2.0"`, "none",
			"MIT", "Apache-2.0", "none", func(c Config) any { return c.License },
		},
	}
	for _, k := range keys {
		t.Run(k.key, func(t *testing.T) {
			line := k.key + " = " + k.file + "\n"
			cases := []struct {
				name string
				body string
				vars map[string]string
				want any
			}{
				{"default", "-", nil, k.def},
				{"file over default", line, nil, k.f},
				{"env over default", "-", map[string]string{k.envVar: k.fromEnv}, k.e},
				{"env over file", line, map[string]string{k.envVar: k.fromEnv}, k.e},
				{"empty env is unset", line, map[string]string{k.envVar: ""}, k.f},
			}
			for _, c := range cases {
				got, err := load(t, c.body, c.vars)
				if err != nil {
					t.Fatalf("%s: %v", c.name, err)
				}
				if v := k.get(got); !reflect.DeepEqual(v, c.want) {
					t.Errorf("%s: %s = %v, want %v", c.name, k.key, v, c.want)
				}
			}
		})
	}
	// --scope is the only flag with a key.
	home := t.TempDir()
	vars := env(map[string]string{"HOME": home, "SKL_SCOPE": "user"})
	p, err := NewPaths(vars)
	if err != nil {
		t.Fatal(err)
	}
	flag, err := ParseScope("project")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Load(context.Background(), p, vars, Flags{Scope: flag})
	if err != nil {
		t.Fatal(err)
	}
	if got.Scope != Project {
		t.Errorf("flag over env: scope = %v", got.Scope)
	}
}

// config.md: "SKL_TARGETS (comma-separated, such as claude,agents)".
func TestTargetsEnv(t *testing.T) {
	tests := []struct {
		in   string
		want []Target
	}{
		{"claude", []Target{Claude}},
		{"agents", []Target{Agents}},
		{"claude,agents", []Target{Claude, Agents}},
		{"agents,claude", []Target{Agents, Claude}},
		{" claude , agents ", []Target{Claude, Agents}},
		{"claude,,agents,", []Target{Claude, Agents}},
		{"claude,claude", []Target{Claude}},
	}
	for _, tt := range tests {
		got, err := load(t, "-", map[string]string{"SKL_TARGETS": tt.in})
		if err != nil {
			t.Errorf("SKL_TARGETS=%q: %v", tt.in, err)
			continue
		}
		if !reflect.DeepEqual(got.Targets, tt.want) {
			t.Errorf("SKL_TARGETS=%q: targets = %v, want %v", tt.in, got.Targets, tt.want)
		}
	}
}

// files.md "Local state", and config.md: "SKL_HOME: Moves the whole ~/.skl
// folder, config file included."
func TestPaths(t *testing.T) {
	home := t.TempDir()
	p, err := NewPaths(env(map[string]string{"HOME": home}))
	if err != nil {
		t.Fatal(err)
	}
	skl := filepath.Join(home, ".skl")
	want := Paths{
		Home:        skl,
		Config:      filepath.Join(skl, "config.toml"),
		Vault:       filepath.Join(skl, "vault"),
		Cache:       filepath.Join(skl, "cache"),
		State:       filepath.Join(skl, "state.json"),
		Generations: filepath.Join(skl, "generations"),
		Lock:        filepath.Join(skl, "lock"),
		Staging:     filepath.Join(skl, "staging"),
	}
	if p != want {
		t.Errorf("NewPaths = %+v, want %+v", p, want)
	}

	team := filepath.Join(t.TempDir(), "skl-team")
	p, err = NewPaths(env(map[string]string{"HOME": home, "SKL_HOME": team}))
	if err != nil {
		t.Fatal(err)
	}
	if p.Home != team || p.Config != filepath.Join(team, "config.toml") || p.Vault != filepath.Join(team, "vault") ||
		p.Lock != filepath.Join(team, "lock") {
		t.Errorf("SKL_HOME: %+v", p)
	}

	// A relative SKL_HOME is made absolute, so it doesn't move with the
	// working directory.
	p, err = NewPaths(env(map[string]string{"SKL_HOME": "rel"}))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(p.Home) || filepath.Base(p.Home) != "rel" {
		t.Errorf("relative SKL_HOME: Home = %q", p.Home)
	}

	if _, err := NewPaths(env(nil)); !errors.Is(err, ErrNoHome) {
		t.Errorf("no HOME, no SKL_HOME: err = %v, want ErrNoHome", err)
	}
}

// SKL_HOME moves the config file too: the file in the old place is not read.
func TestLoadFollowsSKLHome(t *testing.T) {
	home, team := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".skl"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".skl", "config.toml"), []byte(`protocol = "ssh"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(team, "config.toml"), []byte(`scope = "project"`), 0o644); err != nil {
		t.Fatal(err)
	}
	e := env(map[string]string{"HOME": home, "SKL_HOME": team})
	p, err := NewPaths(e)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Load(context.Background(), p, e, Flags{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Protocol != HTTPS || c.Scope != Project {
		t.Errorf("Load with SKL_HOME = %+v, want protocol https, scope project", c)
	}
}

// architecture.md "Third-party parts": "core ignores tables it doesn't own".
// [web] is core, but read by skl open, which comes later.
func TestUnknownTablesIgnored(t *testing.T) {
	body := `protocol = "ssh"

[web]
"git.example.com" = "gitlab"

[skills-sh]
api = "https://skills.sh"
limit = 10

[skills-sh.nested]
x = [1, 2]
`
	got, err := load(t, body, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Protocol != SSH {
		t.Errorf("protocol = %v, want ssh", got.Protocol)
	}
}

// Card #59 "Design": an invalid value is
// "✗ config: <what> · edit <key> in ~/.skl/config.toml".
func TestInvalid(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		vars     map[string]string
		key, msg string
		hint     string
	}{
		{
			"protocol", `protocol = "ftp"`, nil, "protocol",
			`config: protocol must be ssh or https, not "ftp"`, "edit protocol in ~/.skl/config.toml",
		},
		{
			"scope", `scope = "global"`, nil, "scope",
			`config: scope must be user or project, not "global"`, "edit scope in ~/.skl/config.toml",
		},
		{
			"targets unknown", `targets = ["claude", "cursor"]`, nil, "targets",
			`config: targets can only hold claude and agents, not "cursor"`, "edit targets in ~/.skl/config.toml",
		},
		{
			"targets empty", `targets = []`, nil, "targets",
			`config: targets is empty: name claude, agents or both`, "edit targets in ~/.skl/config.toml",
		},
		{
			"license not SPDX", `license = "my own license"`, nil, "license",
			`config: license must be an SPDX id such as MIT, or none, not "my own license"`, "edit license in ~/.skl/config.toml",
		},
		{
			"engine empty", `engine = " "`, nil, "engine",
			`config: engine is empty`, "edit engine in ~/.skl/config.toml",
		},
		{
			"default_host with scheme", `default_host = "https://gitlab.example.com"`, nil, "default_host",
			`config: default_host must be a host name such as github.com, not "https://gitlab.example.com"`,
			"edit default_host in ~/.skl/config.toml",
		},
		{
			"default_host empty", `default_host = ""`, nil, "default_host",
			`config: default_host must be a host name such as github.com, not ""`, "edit default_host in ~/.skl/config.toml",
		},
		// A bad value from the environment names the variable.
		{
			"env protocol", "-",
			map[string]string{"SKL_PROTOCOL": "ftp"},
			"protocol",
			`config: SKL_PROTOCOL must be ssh or https, not "ftp"`, "unset SKL_PROTOCOL",
		},
		{
			"env scope", "-",
			map[string]string{"SKL_SCOPE": "global"},
			"scope",
			`config: SKL_SCOPE must be user or project, not "global"`, "unset SKL_SCOPE",
		},
		{
			"env targets unknown", "-",
			map[string]string{"SKL_TARGETS": "claude,cursor"},
			"targets",
			`config: SKL_TARGETS can only hold claude and agents, not "cursor"`, "unset SKL_TARGETS",
		},
		{
			"env targets empty", "-",
			map[string]string{"SKL_TARGETS": " , "},
			"targets",
			`config: SKL_TARGETS is empty: name claude, agents or both`, "unset SKL_TARGETS",
		},
		{
			"env license", "-",
			map[string]string{"SKL_LICENSE": "MIT License"},
			"license",
			`config: SKL_LICENSE must be an SPDX id such as MIT, or none, not "MIT License"`, "unset SKL_LICENSE",
		},
		{
			"env default_host", "-",
			map[string]string{"SKL_DEFAULT_HOST": "github.com/you"},
			"default_host",
			`config: SKL_DEFAULT_HOST must be a host name such as github.com, not "github.com/you"`, "unset SKL_DEFAULT_HOST",
		},
		// A file that isn't TOML, or a value of the wrong type.
		{
			"not TOML", "protocol = ssh\n", nil, "",
			`config: ~/.skl/config.toml line 1: expected value but found "ssh" instead`, "edit ~/.skl/config.toml",
		},
		{
			"not a string", `protocol = 22`, nil, "protocol",
			`config: protocol must be a string in quotes, not 22`, "edit protocol in ~/.skl/config.toml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(t, tt.body, tt.vars)
			var ie *InvalidError
			if !errors.As(err, &ie) {
				t.Fatalf("err = %v, want *InvalidError", err)
			}
			if ie.Key != tt.key {
				t.Errorf("Key = %q, want %q", ie.Key, tt.key)
			}
			if err.Error() != tt.msg {
				t.Errorf("Error() = %q\n                  want %q", err.Error(), tt.msg)
			}
			if h := ie.Hint(); h != tt.hint {
				t.Errorf("Hint() = %q, want %q", h, tt.hint)
			}
		})
	}
}

// The hint names the file where it is: SKL_HOME moves it, and a home
// outside HOME is shown in full.
func TestInvalidHintFollowsSKLHome(t *testing.T) {
	home, team := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(team, "config.toml"), []byte(`scope = "x"`), 0o644); err != nil {
		t.Fatal(err)
	}
	e := env(map[string]string{"HOME": home, "SKL_HOME": team})
	p, _ := NewPaths(e)
	_, err := Load(context.Background(), p, e, Flags{})
	var ie *InvalidError
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v", err)
	}
	if want := "edit scope in " + filepath.Join(team, "config.toml"); ie.Hint() != want {
		t.Errorf("Hint() = %q, want %q", ie.Hint(), want)
	}

	sub := filepath.Join(home, "skl-team")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "config.toml"), []byte(`scope = "x"`), 0o644); err != nil {
		t.Fatal(err)
	}
	e = env(map[string]string{"HOME": home, "SKL_HOME": sub})
	p, _ = NewPaths(e)
	_, err = Load(context.Background(), p, e, Flags{})
	if !errors.As(err, &ie) || ie.Hint() != "edit scope in ~/skl-team/config.toml" {
		t.Errorf("err = %v", err)
	}
}

func TestLicenseIDs(t *testing.T) {
	for _, id := range []string{"MIT", "Apache-2.0", "GPL-3.0-or-later", "BSD-3-Clause", "MPL-2.0", "LGPL-2.1+", "none", "CC0-1.0"} {
		if _, err := load(t, `license = "`+id+`"`, nil); err != nil {
			t.Errorf("license %q: %v", id, err)
		}
	}
	for _, id := range []string{"", "MIT License", "-MIT", "MIT/Apache", "None", "NONE", "nOnE", "NOASSERTION", "NoAssertion"} {
		if _, err := load(t, `license = "`+id+`"`, nil); err == nil {
			t.Errorf("license %q: no error", id)
		}
	}
}

func TestParseScope(t *testing.T) {
	for in, want := range map[string]Scope{"user": User, "project": Project} {
		if got, err := ParseScope(in); err != nil || got != want {
			t.Errorf("ParseScope(%q) = %v, %v", in, got, err)
		}
	}
	if _, err := ParseScope("global"); err == nil || !strings.Contains(err.Error(), `scope must be user or project, not "global"`) {
		t.Errorf("ParseScope(global) err = %v", err)
	}
}

func TestLoadCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p, _ := NewPaths(env(map[string]string{"HOME": t.TempDir()}))
	if _, err := Load(ctx, p, env(nil), Flags{}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// errorLine is the ✗ line skl prints for a Load error at 80 columns.
func errorLine(t *testing.T, err error) string {
	t.Helper()
	h, ok := err.(interface{ Hint() string })
	if !ok {
		t.Fatalf("err = %#v has no hint", err)
	}
	var out bytes.Buffer
	ui.New(ui.Options{
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &out,
		Getenv: env(map[string]string{"COLUMNS": "80"}),
	}).Error(err.Error(), h.Hint())
	return out.String()
}

// config.md: "A value skl can't use stops the command, naming the variable
// or the key", and commands.md "Output": every ✗ line ends with the command
// to run. Card #79, items 1, 3 and 4.
func TestInvalidLines(t *testing.T) {
	const notNames = "✗ config: targets must be a list of target names\n  · edit targets in ~/.skl/config.toml\n"
	tests := []struct {
		name string
		body string
		vars map[string]string
		want string
	}{
		// Item 1: each list element is one target name.
		{
			"targets element with a comma", `targets = ["claude,agents"]`, nil,
			"✗ config: targets can only hold claude and agents, not \"claude,agents\"\n  · edit targets in ~/.skl/config.toml\n",
		},
		{
			"targets element with spaces", `targets = [" claude "]`, nil,
			"✗ config: targets can only hold claude and agents, not \" claude \"\n  · edit targets in ~/.skl/config.toml\n",
		},
		{
			"targets empty element", `targets = ["claude", ""]`, nil,
			"✗ config: targets can only hold claude and agents, not \"\"\n  · edit targets in ~/.skl/config.toml\n",
		},
		// Item 3: words that say there is no license.
		{
			"license None", `license = "None"`, nil,
			"✗ config: license must be an SPDX id such as MIT, or none, not \"None\"\n  · edit license in ~/.skl/config.toml\n",
		},
		{
			"license NONE", `license = "NONE"`, nil,
			"✗ config: license must be an SPDX id such as MIT, or none, not \"NONE\"\n  · edit license in ~/.skl/config.toml\n",
		},
		{
			"license NOASSERTION", `license = "noassertion"`, nil,
			"✗ config: license must be an SPDX id such as MIT, or none, not \"noassertion\"\n  · edit license in ~/.skl/config.toml\n",
		},
		{
			"SKL_LICENSE NONE", "-",
			map[string]string{"SKL_LICENSE": "NONE"},
			"✗ config: SKL_LICENSE must be an SPDX id such as MIT, or none, not \"NONE\"\n  · unset SKL_LICENSE\n",
		},
		// Item 4: the wrong shape says what was expected.
		{"targets scalar", `targets = "claude"`, nil, notNames},
		{"targets number", `targets = 1`, nil, notNames},
		{"targets table", "[targets]\nx = 1\n", nil, notNames},
		{"targets inline table", `targets = {x = 1}`, nil, notNames},
		{"targets array of tables", "[[targets]]\nx = 1\n", nil, notNames},
		{"targets list of tables", `targets = [{x = 1}]`, nil, notNames},
		{"targets not strings", `targets = ["claude", 1]`, nil, notNames},
		{
			"protocol table", "[protocol]\nx = 1\n", nil,
			"✗ config: protocol must be a string in quotes, not a table\n  · edit protocol in ~/.skl/config.toml\n",
		},
		{
			"engine array of tables", "[[engine]]\nx = 1\n", nil,
			"✗ config: engine must be a string in quotes, not a table\n  · edit engine in ~/.skl/config.toml\n",
		},
		{
			"scope list", `scope = ["user"]`, nil,
			"✗ config: scope must be a string in quotes, not [\"user\"]\n  · edit scope in ~/.skl/config.toml\n",
		},
		{
			"default_host list of tables", `default_host = [{x = 1}]`, nil,
			"✗ config: default_host must be a string in quotes, not [a table]\n  · edit default_host in ~/.skl/config.toml\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(t, tt.body, tt.vars)
			if err == nil {
				t.Fatal("no error")
			}
			if got := errorLine(t, err); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}
}

// config.md "Precedence": "Only the value that wins is checked: a bad one in
// the file doesn't matter while a variable or a flag overrides it." Card
// #79, item 2: for its type as well as its value.
func TestOverriddenNotChecked(t *testing.T) {
	tests := []struct {
		key, envVar, good string
		bad               []string // TOML values
		get               func(Config) any
		want              any
	}{
		{
			"default_host", "SKL_DEFAULT_HOST", "git.example.org",
			[]string{`"https://x"`, `22`, `{x = 1}`},
			func(c Config) any { return c.DefaultHost }, "git.example.org",
		},
		{
			"protocol", "SKL_PROTOCOL", "ssh",
			[]string{`"ftp"`, `22`, `["ssh"]`, `{x = 1}`},
			func(c Config) any { return c.Protocol }, SSH,
		},
		{
			"scope", "SKL_SCOPE", "project",
			[]string{`"global"`, `22`, `{x = 1}`},
			func(c Config) any { return c.Scope }, Project,
		},
		{
			"targets", "SKL_TARGETS", "agents",
			[]string{`["cursor"]`, `"claude"`, `22`, `{x = 1}`, `[]`},
			func(c Config) any { return c.Targets },
			[]Target{Agents},
		},
		{
			"engine", "SKL_ENGINE", "pi {prompt}",
			[]string{`" "`, `22`, `{x = 1}`},
			func(c Config) any { return c.Engine }, "pi {prompt}",
		},
		{
			"license", "SKL_LICENSE", "MIT",
			[]string{`"my own"`, `"None"`, `22`, `{x = 1}`},
			func(c Config) any { return c.License }, "MIT",
		},
	}
	for _, tt := range tests {
		for _, bad := range tt.bad {
			t.Run(tt.key+" "+bad, func(t *testing.T) {
				body := tt.key + " = " + bad + "\n"
				got, err := load(t, body, map[string]string{tt.envVar: tt.good})
				if err != nil {
					t.Fatalf("overridden by %s: %v", tt.envVar, err)
				}
				if v := tt.get(got); !reflect.DeepEqual(v, tt.want) {
					t.Errorf("%s = %v, want %v", tt.key, v, tt.want)
				}
				// Without the variable, the same value stops the command.
				if _, err := load(t, body, nil); err == nil {
					t.Errorf("not overridden: no error")
				}
			})
		}
	}

	// The flag overrides both, so neither is checked.
	home := t.TempDir()
	e := env(map[string]string{"HOME": home, "SKL_SCOPE": "global"})
	p, err := NewPaths(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(p.Home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Config, []byte("scope = 22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(context.Background(), p, e, Flags{Scope: User})
	if err != nil || got.Scope != User {
		t.Errorf("--scope over a bad variable and file: %v, %v", got.Scope, err)
	}
	if _, err := Load(context.Background(), p, e, Flags{}); err == nil {
		t.Error("no flag: a bad SKL_SCOPE was not checked")
	}

	// A file that isn't TOML has no value to override: it still stops.
	if _, err := load(t, "protocol = ssh\n", map[string]string{"SKL_PROTOCOL": "https"}); err == nil {
		t.Error("a file that doesn't parse: no error")
	}
}

// Card #79, item 5: "✗ can't read ~/.skl/config.toml: <reason> · check its
// permissions".
func TestUnreadable(t *testing.T) {
	setup := func(t *testing.T) (Paths, func(string) string) {
		t.Helper()
		e := env(map[string]string{"HOME": t.TempDir()})
		p, err := NewPaths(e)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(p.Home, 0o755); err != nil {
			t.Fatal(err)
		}
		return p, e
	}
	tests := []struct {
		name  string
		setup func(t *testing.T, p Paths)
		want  string
	}{
		{
			"a folder where the file goes",
			func(t *testing.T, p Paths) {
				t.Helper()
				if err := os.Mkdir(p.Config, 0o755); err != nil {
					t.Fatal(err)
				}
			},
			"✗ can't read ~/.skl/config.toml: is a directory · check its permissions\n",
		},
		{
			"no permission",
			func(t *testing.T, p Paths) {
				t.Helper()
				if os.Getuid() == 0 {
					t.Skip("root reads everything")
				}
				if err := os.WriteFile(p.Config, nil, 0o000); err != nil {
					t.Fatal(err)
				}
			},
			"✗ can't read ~/.skl/config.toml: permission denied · check its permissions\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, e := setup(t)
			tt.setup(t, p)
			_, err := Load(context.Background(), p, e, Flags{})
			if !errors.As(err, new(*UnreadableError)) {
				t.Fatalf("err = %v, want *UnreadableError", err)
			}
			if got := errorLine(t, err); got != tt.want {
				t.Errorf("got  %q\nwant %q", got, tt.want)
			}
		})
	}

	t.Run("the cause can be checked", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("root reads everything")
		}
		p, e := setup(t)
		if err := os.WriteFile(p.Config, nil, 0o000); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(context.Background(), p, e, Flags{}); !errors.Is(err, fs.ErrPermission) {
			t.Errorf("err = %v, want fs.ErrPermission", err)
		}
	})
}
