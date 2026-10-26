package ansi

import (
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// RequestProgramStatus queries OSC 7501 support and accepted state and kind names.
const RequestProgramStatus = "\x1b]7501;?\x1b\\"

// ProgramStatusState is the state of a program or one of its tasks.
// Convert a string to this type for an extension name. The writer validates it.
type ProgramStatusState string

const (
	// ProgramStatusIdle means the program is ready for the next instruction.
	ProgramStatusIdle ProgramStatusState = "idle"
	// ProgramStatusWorking means work is in progress.
	ProgramStatusWorking ProgramStatusState = "working"
	// ProgramStatusDone means work finished and its result is ready.
	ProgramStatusDone ProgramStatusState = "done"
	// ProgramStatusBlocked means work needs user input before it can continue.
	ProgramStatusBlocked ProgramStatusState = "blocked"
	// ProgramStatusError means work failed and stopped.
	ProgramStatusError ProgramStatusState = "error"
	// ProgramStatusClear removes the addressed record and its descendants.
	ProgramStatusClear ProgramStatusState = "clear"
)

// String returns the state name.
func (s ProgramStatusState) String() string { return string(s) }

// ProgramStatusKind describes what a blocked program needs from the user.
// Convert a string to this type for an extension name. The writer validates it.
type ProgramStatusKind string

const (
	// ProgramStatusPermission requests approval for an action.
	ProgramStatusPermission ProgramStatusKind = "permission"
	// ProgramStatusQuestion requests an answer to a question.
	ProgramStatusQuestion ProgramStatusKind = "question"
	// ProgramStatusAuth requests authentication.
	ProgramStatusAuth ProgramStatusKind = "auth"
)

// String returns the kind name.
func (k ProgramStatusKind) String() string { return string(k) }

// ProgramStatusCapabilities contains the names accepted by an OSC 7501 terminal.
// An empty list accepts no names. Clear is an operation, not an advertised state.
type ProgramStatusCapabilities struct {
	// States contains accepted state names, including extensions.
	States []ProgramStatusState
	// Kinds contains accepted blocked-kind names, including extensions.
	Kinds []ProgramStatusKind
}

// ProgramStatus is a complete OSC 7501 report. Its zero value is invalid.
//
// Each report replaces its record. Include every field that should remain.
// A nil ID selects the root. A root clear removes all records.
// The terminal controls record lifetime; done and error records can survive exit.
// Text fields contain plain UTF-8, not base64. Nil fields are absent.
type ProgramStatus struct {
	// State is the required record state.
	State ProgramStatusState
	// ID is a path of at most 128 bytes and eight slash-separated segments.
	// Each segment contains 1-32 bytes from [A-Za-z0-9_.+-].
	ID *string
	// Kind is the reason for a blocked state.
	Kind *ProgramStatusKind
	// Progress is 0-100 for working or blocked states. Nil means indeterminate.
	Progress *int
	// App contains 1-32 bytes from [A-Za-z0-9_.+-].
	// An absent app inherits from the nearest ancestor that has one.
	App *string
	// Title is a label of at most 192 UTF-8 bytes, without control characters.
	Title *string
	// Message contains at most 2048 UTF-8 bytes, without control characters.
	Message *string
}

// WriteProgramStatus writes a complete OSC 7501 report without flushing.
//
// It validates every field before output. Invalid fields return an error that
// wraps [fs.ErrInvalid]. Text limits count UTF-8 bytes before base64 encoding.
// State and kind names must be nonempty values from [A-Za-z0-9_.,+/=-].
// The complete encoded sequence, including framing, must fit in 4096 bytes.
// Extension names do not establish support; applications select any fallback.
// The return values describe bytes written and errors, not terminal acceptance.
//
// See https://www.superlogical.com/rex/docs/build/program-status.
func WriteProgramStatus(w io.Writer, report ProgramStatus) (int, error) {
	invalid := func(field string) (int, error) {
		return 0, fmt.Errorf("ansi: invalid program status %s: %w", field, fs.ErrInvalid)
	}
	if !validStatusName(report.State.String()) {
		return invalid("state")
	}
	if report.Kind != nil && !validStatusName(report.Kind.String()) {
		return invalid("kind")
	}
	if report.ID != nil {
		parts := strings.Split(*report.ID, "/")
		if len(*report.ID) > 128 || len(parts) > 8 {
			return invalid("id")
		}
		for _, part := range parts {
			if !validStatusSegment(part) {
				return invalid("id")
			}
		}
	}
	if report.App != nil && !validStatusSegment(*report.App) {
		return invalid("app")
	}
	if report.Progress != nil && (*report.Progress < 0 || *report.Progress > 100) {
		return invalid("progress")
	}
	if report.Title != nil && !validStatusText(*report.Title, 192) {
		return invalid("title")
	}
	if report.Message != nil && !validStatusText(*report.Message, 2048) {
		return invalid("message")
	}

	var b strings.Builder
	b.WriteString("\x1b]7501;state=")
	b.WriteString(report.State.String())
	if report.ID != nil {
		b.WriteString(":id=")
		b.WriteString(*report.ID)
	}
	if report.Kind != nil {
		b.WriteString(":kind=")
		b.WriteString(report.Kind.String())
	}
	if report.Progress != nil {
		b.WriteString(":progress=")
		b.WriteString(strconv.Itoa(*report.Progress))
	}
	if report.App != nil {
		b.WriteString(":app=")
		b.WriteString(*report.App)
	}
	if report.Title != nil {
		b.WriteString(":title=")
		b.WriteString(base64.StdEncoding.EncodeToString([]byte(*report.Title)))
	}
	if report.Message != nil {
		b.WriteString(":msg=")
		b.WriteString(base64.StdEncoding.EncodeToString([]byte(*report.Message)))
	}
	b.WriteString("\x1b\\")
	if b.Len() > 4096 {
		return invalid("sequence length")
	}
	n, err := io.WriteString(w, b.String())
	if err == nil && n != b.Len() {
		err = io.ErrShortWrite
	}
	return n, err
}

func validStatusName(s string) bool {
	return len(s) > 0 && len(s) <= 4096 &&
		strings.Trim(s, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_.,+/=-") == ""
}

func validStatusSegment(s string) bool {
	if len(s) < 1 || len(s) > 32 {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_.+-", rune(c))) {
			return false
		}
	}
	return true
}

func validStatusText(s string, limit int) bool {
	if len(s) > limit || !utf8.ValidString(s) {
		return false
	}
	for _, c := range s {
		if unicode.IsControl(c) {
			return false
		}
	}
	return true
}
