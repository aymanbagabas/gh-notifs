package ansi

import "strings"

// Notify sends a simple notification with ESC ] 9 ; body BEL.
func Notify(body string) string { return "\x1b]9;" + body + "\x07" }

// DesktopNotification sends a metadata-bearing notification with ESC ] 99 ; metadata ; body BEL.
func DesktopNotification(body string, metadata ...string) string {
	return "\x1b]99;" + strings.Join(metadata, ":") + ";" + body + "\x07"
}
