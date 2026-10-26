//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || zos

package program

import (
	"os"
	"os/signal"

	"golang.org/x/sys/unix"
)

// suspendSelf stops the process group and returns once it is continued.
//
// The signal goes to the whole group rather than to this process alone, which
// is what group zero means. A program is rarely the only process in its group:
// under `go run` the parent stays in the same group, and stopping only this
// process leaves that parent running, so the shell never sees the job stop and
// never gives the terminal back.
//
// Waiting for SIGCONT is what makes the call return at the right moment. Kill
// returns as soon as the signal is queued rather than when the process has
// stopped, so without the wait a caller would run its resume path while the
// stop was still in flight, and restore a terminal it is about to lose.
var suspendSelf = func() error {
	cont := make(chan os.Signal, 1)
	signal.Notify(cont, unix.SIGCONT)
	defer signal.Stop(cont)

	if err := unix.Kill(0, unix.SIGTSTP); err != nil {
		return err
	}
	<-cont
	return nil
}

// Suspend pauses the program and stops the process with SIGTSTP.
//
// It returns once the process has been continued, so the caller resumes only
// after the terminal belongs to it again:
//
//	if err := p.Suspend(); err != nil {
//		return err
//	}
//	return p.Resume()
func (p *Program) Suspend() error {
	if err := p.Pause(); err != nil {
		return err
	}
	return suspendSelf()
}
