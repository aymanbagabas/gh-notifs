Output bin/inbox-frames/

Set Shell "bash"
Set FontFamily "Menlo"
Set FontSize 20
Set Columns 120
Set Rows 36
Set Padding 20
Set Theme "GitHub Dark"
Set Framerate 12
Set TypingSpeed 70ms
Set CursorBlink false

Env TERM "xterm-256color"
Env COLORTERM "truecolor"

Hide
Type "./bin/inbox-demo"
Enter
Wait+Screen /6 loaded/
Type "v"
Wait+Screen /What changed/
Show

# Pauses let viewers read; Wait commands check application state.
Sleep 3s
Type "m"
Sleep 2s
Type "v"
Type "j"
Sleep 800ms
Type "x"
Sleep 1s
Type "xk"

Type "/reason:"
Wait+Screen /review-requested/
Sleep 1s
Type "review-requested"
Enter
Wait+Screen /2 loaded/
Sleep 2s

Type "/"
Ctrl+u
Enter
Wait+Screen /6 loaded/
Tab
Down
Enter
Wait+Screen /3 loaded/
Sleep 2s
Tab
Up
Enter
Wait+Screen /6 loaded/

Type "RB"
Wait+Screen /Auto:on/
Sleep 2s
Type "RB"
Type "v"
Wait+Screen /What changed/
Type "m"
Sleep 3s

Hide
Type "q"
