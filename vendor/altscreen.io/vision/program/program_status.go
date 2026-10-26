package program

import "altscreen.io/vision/ansi"

// ProgramStatus is a complete OSC 7501 report.
type ProgramStatus = ansi.ProgramStatus

// ProgramStatusState is the state of a program or task.
type ProgramStatusState = ansi.ProgramStatusState

// ProgramStatusKind describes what a blocked program needs.
type ProgramStatusKind = ansi.ProgramStatusKind

const (
	// ProgramStatusIdle means the program is ready for the next instruction.
	ProgramStatusIdle = ansi.ProgramStatusIdle
	// ProgramStatusWorking means work is in progress.
	ProgramStatusWorking = ansi.ProgramStatusWorking
	// ProgramStatusDone means work finished and its result is ready.
	ProgramStatusDone = ansi.ProgramStatusDone
	// ProgramStatusBlocked means work needs user input.
	ProgramStatusBlocked = ansi.ProgramStatusBlocked
	// ProgramStatusError means work failed and stopped.
	ProgramStatusError = ansi.ProgramStatusError
	// ProgramStatusClear removes the addressed record and its descendants.
	ProgramStatusClear = ansi.ProgramStatusClear
	// ProgramStatusPermission requests approval for an action.
	ProgramStatusPermission = ansi.ProgramStatusPermission
	// ProgramStatusQuestion requests an answer to a question.
	ProgramStatusQuestion = ansi.ProgramStatusQuestion
	// ProgramStatusAuth requests authentication.
	ProgramStatusAuth = ansi.ProgramStatusAuth
)

// SetProgramStatus sends a complete OSC 7501 report and flushes.
//
// Reports replace their records. Include every field that should remain.
// Clear removes the record and descendants; a root clear removes all records.
// The terminal controls record lifetime. Pause, resume, screen switches, and
// Finish leave reports unchanged, so done and error records can survive exit.
// Unsupported terminals ignore reports. Discovery is optional.
// Invalid fields return an error before output; see [ansi.WriteProgramStatus].
func (p *Program) SetProgramStatus(report ProgramStatus) error {
	if _, err := ansi.WriteProgramStatus(p.screen, report); err != nil {
		return err
	}
	return p.screen.Flush()
}

// RequestProgramStatus queries OSC 7501 support and flushes without waiting.
// A [altscreen.io/vision/event.ProgramStatusEvent] reply updates
// [Capabilities.ProgramStatus].
// Options.QueryProgramStatus includes this query in QueryCapabilities.
func (p *Program) RequestProgramStatus() error {
	return p.writeFlush(ansi.RequestProgramStatus)
}
