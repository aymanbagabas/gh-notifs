package term

import "os"

// OpenTTY opens the controlling terminal for reading and writing.
//
// Both halves refer to the same device, opened separately so each can be
// closed independently. On Unix that device is /dev/tty; on Windows it is the
// CONIN$ and CONOUT$ console pair.
//
// It fails when the process has no controlling terminal, which is what
// happens under a daemon, a cron job, or a build system.
func OpenTTY() (input, output *os.File, err error) {
	input, err = os.OpenFile(ttyInputName, os.O_RDWR, 0)
	if err != nil {
		return nil, nil, err
	}
	output, err = os.OpenFile(ttyOutputName, os.O_RDWR, 0)
	if err != nil {
		input.Close()
		return nil, nil, err
	}
	return input, output, nil
}
