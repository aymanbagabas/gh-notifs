package ansi

// WindowTitleAndIcon sets the window title and icon name with ESC ] 0 ; title ESC \.
func WindowTitleAndIcon(title string) string { return "\x1b]0;" + title + "\x1b\\" }

// IconName sets the icon name with ESC ] 1 ; name ESC \.
func IconName(name string) string { return "\x1b]1;" + name + "\x1b\\" }

// WindowTitle sets the window title with ESC ] 2 ; title ESC \.
func WindowTitle(title string) string { return "\x1b]2;" + title + "\x1b\\" }
