package term

import (
	"os"
	"strings"
)

// Environ is a read-only view of a set of environment variables.
//
// Implementations decide where the variables come from and whether they can
// change: [ProcessEnv] reads the live process environment on every lookup,
// while [Env] answers from a fixed list.
//
// The two methods mirror [os.Getenv] and [os.LookupEnv], and must agree with
// each other: Getenv returns the empty string exactly when LookupEnv reports
// the variable absent, or when it is present and empty.
type Environ interface {
	// Getenv returns the value of the variable named by key. It returns the
	// empty string if the variable is absent, which is indistinguishable
	// from a variable present with an empty value. Use LookupEnv to tell
	// them apart.
	Getenv(key string) string

	// LookupEnv returns the value of the variable named by key and whether
	// it was present.
	LookupEnv(key string) (string, bool)
}

// ProcessEnv is the live process environment.
//
// Every lookup reads through to the os package, so a variable changed after
// this value was created is visible to the next lookup. This is the
// environment a [Terminal] uses when built over process stdio or the
// controlling terminal.
//
// Two things follow from going through the os package that are not true of
// [Env]: a value that is not valid UTF-8 is returned as-is rather than being
// treated as absent, and on Windows names match case-insensitively.
type ProcessEnv struct{}

// Getenv returns the value of the variable named by key, or the empty string
// if it is absent.
func (ProcessEnv) Getenv(key string) string { return os.Getenv(key) }

// LookupEnv returns the value of the variable named by key and whether it was
// present.
func (ProcessEnv) LookupEnv(key string) (string, bool) { return os.LookupEnv(key) }

// Env is a fixed list of environment variables, each in the "KEY=VALUE" form
// used by [os.Environ].
//
// Use it for an environment that does not come from this process, such as the
// variables an SSH client forwards or a set read from a configuration file,
// and for tests that need deterministic lookups.
//
// Duplicate keys are allowed and the last match wins, matching how a process
// environment behaves at the exec boundary. Entries with no "=" are ignored.
// Names match exactly, including on Windows.
type Env []string

// Getenv returns the value of the variable named by key, or the empty string
// if it is absent.
func (e Env) Getenv(key string) string {
	v, _ := e.LookupEnv(key)
	return v
}

// LookupEnv returns the value of the variable named by key and whether it was
// present. When key appears more than once, the last entry wins.
func (e Env) LookupEnv(key string) (string, bool) {
	for i := len(e) - 1; i >= 0; i-- {
		name, value, ok := strings.Cut(e[i], "=")
		if ok && name == key {
			return value, true
		}
	}
	return "", false
}

// Set appends a variable, shadowing any earlier entry with the same name.
func (e *Env) Set(key, value string) {
	*e = append(*e, key+"="+value)
}

// ProcessEnviron returns a snapshot of the current process environment.
//
// Later changes to the process environment are not reflected in it. Use
// [ProcessEnv] to read through to the live environment instead.
func ProcessEnviron() Env {
	return Env(os.Environ())
}

// HasEnv reports whether key is present in e with a non-empty value.
//
// A variable set to the empty string counts as absent, because terminals
// treat an empty setting as unset.
func HasEnv(e Environ, key string) bool {
	return e.Getenv(key) != ""
}
