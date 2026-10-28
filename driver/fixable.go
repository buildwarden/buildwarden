package driver

import "fmt"

// FixableError is a failure whose cause is known and which the user can
// fix: a missing privilege, an unprepared image, an absent tool, a bad flag.
// warden prints it with its Fix and without the file-a-bug link, because it
// is not a bug.
//
// The project standard (UX-04 in docs/development/ux-cli-config.md): when a
// failure has a clear cause, detect it as early as is cheap, before any slow
// or irreversible work, and return a FixableError naming the exact command
// or change that resolves it.
type FixableError struct {
	// Msg states what is wrong.
	Msg string
	// Fix states what to do about it, ideally a command to run. Optional
	// when Msg already says it.
	Fix string
}

func (e *FixableError) Error() string { return e.Msg }

// Fixable returns a FixableError whose message is formatted from format and
// args, with fix as the remedy.
func Fixable(fix, format string, args ...any) error {
	return &FixableError{Msg: fmt.Sprintf(format, args...), Fix: fix}
}
