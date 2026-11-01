package selector

import (
	"errors"
	"testing"
)

// The sentences are from docs/reference/repos.md "Grammar".
func TestParse(t *testing.T) {
	tests := []struct {
		sentence string
		word     string
		want     Selector
	}{
		{"One part is a group", "review", Selector{Kind: Group, Group: "review"}},
		{"two parts are a skill in a group", "review/pr-review", Selector{Kind: Skill, Group: "review", Name: "pr-review"}},
		{"@name is a named skill set in sets/", "@backend", Selector{Kind: Set, Name: "backend"}},
		{"- removes: -review/test-gaps", "-review/test-gaps", Selector{Remove: true, Kind: Skill, Group: "review", Name: "test-gaps"}},
		{"- removes: -review", "-review", Selector{Remove: true, Kind: Group, Group: "review"}},
		{"- removes: -@base", "-@base", Selector{Remove: true, Kind: Set, Name: "base"}},
		{"name: letters, digits, - and _", "a_1-b/C2_d-e", Selector{Kind: Skill, Group: "a_1-b", Name: "C2_d-e"}},
		{"name: starting with a digit", "1a", Selector{Kind: Group, Group: "1a"}},
		{"name: letters beyond ASCII", "résumé", Selector{Kind: Group, Group: "résumé"}},
		{"reserved names are for groups: a skill", "review/sets", Selector{Kind: Skill, Group: "review", Name: "sets"}},
		{"reserved names are for groups: a skill set", "@h", Selector{Kind: Set, Name: "h"}},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got, err := Parse(tt.word)
			if err != nil {
				t.Fatalf("%s: Parse(%q): %v", tt.sentence, tt.word, err)
			}
			if got != tt.want {
				t.Errorf("%s: Parse(%q) = %+v, want %+v", tt.sentence, tt.word, got, tt.want)
			}
			if s := got.String(); s != tt.word {
				t.Errorf("String() = %q, want %q", s, tt.word)
			}
		})
	}
}

// "Anything else is a reference: three or more parts, a dot in the first
// part, a URL (https://…, git@host:…), or a word that starts with ~, / or ."
func TestParseReference(t *testing.T) {
	tests := []struct{ word, ref string }{
		{"someone/agent-skills/pdf", ""},
		{"someone/agent-skills/@backend", ""},
		{"a/b/c/d", ""},
		{"someone/pdf-skill//", ""},
		{"gitlab.com/team/skills", ""},
		{"gitlab.com/skills", ""},
		{"example.com", ""},
		{"https://git.example.com/team/skills.git", ""},
		{"ssh://git@git.example.com/team/skills", ""},
		{"git@git.example.com:team/skills.git", ""},
		{"git@host:skills", ""},
		{"~/code/skills", ""},
		{"~", ""},
		{"/srv/skills", ""},
		{"./skills", ""},
		{"../skills", ""},
		{".", ""},
		{".hidden", ""},
		// "#" only pins a reference (repos.md "Pinning a version").
		{"someone/pdf-skill#v1.2", ""},
		{"someone/agent-skills/pdf#v1.2", ""},
		// "-" removes "what the rest of the selector names": the rest is what
		// the user should learn.
		{"-someone/agent-skills/pdf", "someone/agent-skills/pdf"},
		{"-https://example.com/a", "https://example.com/a"},
		{"-~/code/skills", "~/code/skills"},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			want := tt.ref
			if want == "" {
				want = tt.word
			}
			_, err := Parse(tt.word)
			if !errors.Is(err, ErrReference) {
				t.Fatalf("Parse(%q) = %v, want ErrReference", tt.word, err)
			}
			if errors.Is(err, ErrInvalid) {
				t.Errorf("Parse(%q): a reference is not also ErrInvalid", tt.word)
			}
			var ref *ReferenceError
			if !errors.As(err, &ref) {
				t.Fatalf("Parse(%q) = %T, want *ReferenceError", tt.word, err)
			}
			if ref.Ref != want {
				t.Errorf("Parse(%q): Ref = %q, want %q", tt.word, ref.Ref, want)
			}
			// skill-sets.md "Selectors": ✗ … is someone else's skill.
			if got, msg := err.Error(), want+" is someone else's skill"; got != msg {
				t.Errorf("Parse(%q): message %q, want %q", tt.word, got, msg)
			}
		})
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		sentence string
		word     string
	}{
		{"name: not empty", ""},
		{"selector: - needs something to remove", "-"},
		{"selector: @ needs a name", "@"},
		{"selector: -@ needs a name", "-@"},
		{"name: starting with a letter or a digit, not -", "--review"},
		{"name: starting with a letter or a digit, not _", "_review"},
		{"name: starting with a letter or a digit, in a skill", "review/-x"},
		{"name: starting with a letter or a digit, in a set", "@-x"},
		{"Every name is free of dots: a skill", "review/pr.review"},
		{"Every name is free of dots: a set", "@back.end"},
		{"name: no other characters", "re view"},
		{"name: no other characters, in a skill", "review/a:b"},
		{"group / skill: both parts", "review/"},
		{"@ only starts a set, one part", "@a/b"},
		{"@ only starts a set, not the second part", "review/@base"},
		{"A group can't be named sets", "sets"},
		{"A group can't be named lessons", "lessons"},
		{"A group can't be named h", "h"},
		{"A group can't be named sets: a skill in it", "sets/backend"},
		{"A group can't be named lessons: a skill in it", "lessons/x"},
		{"A group can't be named h: a skill in it", "h/x"},
		{"A group can't be named sets: removing it", "-sets"},
		{"-h is help, never a removal", "-h"},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			got, err := Parse(tt.word)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("%s: Parse(%q) = %+v, %v, want ErrInvalid", tt.sentence, tt.word, got, err)
			}
			if errors.Is(err, ErrReference) {
				t.Errorf("%s: Parse(%q): an invalid word is not also ErrReference", tt.sentence, tt.word)
			}
		})
	}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{
		"a": true, "A1": true, "1-a_b": true, "é": true,
		"": false, "-a": false, "_a": false, "a.b": false, "a/b": false, "a b": false, "@a": false,
	} {
		if got := ValidName(name); got != want {
			t.Errorf("ValidName(%q) = %v, want %v", name, got, want)
		}
	}
}

// "When a new skill's group would be the owner's name and that name is
// reserved or isn't a valid name, skl uses <owner>-skills instead."
func TestOwnerGroup(t *testing.T) {
	tests := []struct {
		owner, want string
		wantErr     bool
	}{
		{owner: "someone", want: "someone"},
		{owner: "some_one-2", want: "some_one-2"},
		{owner: "sets", want: "sets-skills"},
		{owner: "lessons", want: "lessons-skills"},
		{owner: "h", want: "h-skills"},
		// GitHub allows neither of these, but a local repo's folder can be one.
		{owner: "-x", wantErr: true},
		{owner: "my.org", wantErr: true},
		{owner: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.owner, func(t *testing.T) {
			got, err := OwnerGroup(tt.owner)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("OwnerGroup(%q) = %q, %v, want ErrInvalid", tt.owner, got, err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("OwnerGroup(%q) = %q, %v, want %q", tt.owner, got, err, tt.want)
			}
		})
	}
}
