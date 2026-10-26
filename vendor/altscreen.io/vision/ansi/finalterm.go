package ansi

import (
	"strconv"
	"strings"
)

// FinalTerm emits a generic OSC 133 marker with ESC ] 133 [;param...] BEL.
func FinalTerm(params ...string) string {
	var b strings.Builder
	b.WriteString("\x1b]133")
	for _, p := range params {
		b.WriteByte(';')
		b.WriteString(p)
	}
	b.WriteByte('\x07')
	return b.String()
}

// PromptStart marks the start of a prompt: ESC ] 133 ; A BEL.
func PromptStart() string { return "\x1b]133;A\x07" }

// CommandStart marks the end of the prompt and start of command input: ESC ] 133 ; B BEL.
func CommandStart() string { return "\x1b]133;B\x07" }

// CommandExecuted marks the moment command execution begins: ESC ] 133 ; C BEL.
func CommandExecuted() string { return "\x1b]133;C\x07" }

// CommandFinished marks command completion with ESC ] 133 ; D [;exit-code] BEL.
func CommandFinished(exitCode ...int) string {
	if len(exitCode) == 0 {
		return "\x1b]133;D\x07"
	}
	return "\x1b]133;D;" + strconv.Itoa(exitCode[0]) + "\x07"
}
