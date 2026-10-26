package ansi

// NotifyWorkingDirectory notifies the terminal of the current working directory with ESC ] 7 ; url BEL.
func NotifyWorkingDirectory(url string) string { return "\x1b]7;" + url + "\x07" }
