package gitx

import "regexp"

// userinfoPassword is the user and password in a URL's authority:
// scheme://user:password@. The password ends at the last @ before the host;
// a slash or whitespace ends the URL's authority first.
var userinfoPassword = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://[^\s/?#@:'"]*):[^\s/?#'"]*@`)

// HidePasswords replaces the password of every URL in s with xxxxx, keeping
// the user, as ref.HidePassword does for a word (docs/reference/repos.md "How
// a reference is stored"). skl runs it on what it prints from git: each
// command line SKL_DEBUG=1 shows and git's standard error.
func HidePasswords(s string) string {
	return userinfoPassword.ReplaceAllString(s, "${1}:xxxxx@")
}
