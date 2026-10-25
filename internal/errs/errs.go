// Package errs holds the two error types that reach the user: an error with
// the command that deals with it, and a usage error. Domain packages return
// plain wrapped errors; only the command layer wraps them here.
package errs

import "fmt"

// Hinted is an error with the command to run next. It prints as
// "✗ <Err> · <Hint>" and exits with code 1.
type Hinted struct {
	Err  error
	Hint string
}

func (e *Hinted) Error() string { return e.Err.Error() }
func (e *Hinted) Unwrap() error { return e.Err }

// Hint wraps err with the command that deals with it.
func Hint(err error, hint string) error {
	return &Hinted{Err: err, Hint: hint}
}

// Usage is a mistake in the command line: an unknown command or flag, a
// missing argument, a word that can't be parsed. It exits with code 2, and
// nothing has changed.
type Usage struct {
	Err  error
	Hint string
}

func (e *Usage) Error() string { return e.Err.Error() }
func (e *Usage) Unwrap() error { return e.Err }

// Usagef returns a usage error with a hint and a formatted message.
func Usagef(hint, format string, a ...any) error {
	return &Usage{Err: fmt.Errorf(format, a...), Hint: hint}
}
