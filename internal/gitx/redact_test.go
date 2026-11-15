package gitx

import "testing"

// docs/reference/repos.md "How a reference is stored": "skl drops the password
// and keeps the user". What skl prints from git follows the same rule.
func TestHidePasswords(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"clone -- https://you:s3cret@git.example.com/you/skills /tmp/v",
			"clone -- https://you:xxxxx@git.example.com/you/skills /tmp/v"},
		{"fatal: unable to access 'https://you:s3cret@git.example.com/you/skills/': refused",
			"fatal: unable to access 'https://you:xxxxx@git.example.com/you/skills/': refused"},
		{"ssh://git:tok@git.example.com:2222/team/skills", "ssh://git:xxxxx@git.example.com:2222/team/skills"},
		{"https://you:p@ss@git.example.com/x", "https://you:xxxxx@git.example.com/x"}, // the last @ ends it
		{"https://you:@git.example.com/x", "https://you:xxxxx@git.example.com/x"},
		{"a https://u:1@h/x and http://v:2@k/y", "a https://u:xxxxx@h/x and http://v:xxxxx@k/y"},
		// No password: nothing changes.
		{"https://you@git.example.com/you/skills", "https://you@git.example.com/you/skills"},
		{"ssh://git@git.example.com:2222/team/skills", "ssh://git@git.example.com:2222/team/skills"},
		{"https://git.example.com:443/team/skills@v1", "https://git.example.com:443/team/skills@v1"},
		{"git@git.example.com:team/skills.git", "git@git.example.com:team/skills.git"},
		{"ls-remote --symref -- origin", "ls-remote --symref -- origin"},
	} {
		if got := HidePasswords(tt.in); got != tt.want {
			t.Errorf("HidePasswords(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
