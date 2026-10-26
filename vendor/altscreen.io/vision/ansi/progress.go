package ansi

import "strconv"

// ResetProgressBar resets or hides progress: ESC ] 9 ; 4 ; 0 BEL.
const ResetProgressBar = "\x1b]9;4;0\x07"

// SetIndeterminateProgressBar sets indeterminate progress: ESC ] 9 ; 4 ; 3 BEL.
const SetIndeterminateProgressBar = "\x1b]9;4;3\x07"

func clampPercentage(p int) int {
	switch {
	case p < 0:
		return 0
	case p > 100:
		return 100
	default:
		return p
	}
}

func progressBar(state, percentage int) string {
	return "\x1b]9;4;" + strconv.Itoa(state) + ";" + strconv.Itoa(clampPercentage(percentage)) + "\x07"
}

// SetProgressBar sets normal progress with ESC ] 9 ; 4 ; 1 ; percentage BEL.
func SetProgressBar(percentage int) string { return progressBar(1, percentage) }

// SetErrorProgressBar sets error progress with ESC ] 9 ; 4 ; 2 ; percentage BEL.
func SetErrorProgressBar(percentage int) string { return progressBar(2, percentage) }

// SetWarningProgressBar sets warning progress with ESC ] 9 ; 4 ; 4 ; percentage BEL.
func SetWarningProgressBar(percentage int) string { return progressBar(4, percentage) }
