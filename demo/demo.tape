# Record the demo with fake data: run "vhs demo/demo.tape" in the root of the
# repository. It needs VHS 0.12.1 or later, jq, fzf, and gum. demo/gh is a fake
# gh that answers from demo/data.json.

Output demo/demo.gif

Require jq
Require fzf
Require gum

Set Shell bash
Set FontFamily "JetBrains Mono"
Set FontSize 16
Set Width 1280
Set Height 720
Set Padding 20
Set Theme "GitHub Dark"
Set WindowBar Colorful
Set BorderRadius 8
Set CursorBlink false
Set TypingSpeed 50ms
Set Framerate 24

Hide
Type `export PATH="$PWD/demo:$PATH" LESS=FRX; unset NO_COLOR GH_EXTENSION GH_PATH; clear`
Enter
Show

# Choose in fzf, with a preview. Type to filter, move down, and press enter to
# open. fzf keeps the cursor row when the query changes, so type first.
Type "gh notifs"
Sleep 500ms
Enter
Sleep 3s
Type "retry"
Sleep 2s
Down
Sleep 2s
Enter
Sleep 2.5s

# Print the items that you opened, read or not.
Type "gh notifs list -A --me"
Sleep 500ms
Enter
Sleep 3.5s

# Pipe the tab-separated fields to another picker.
Type "gh notifs list --review-requested | cut -f5,6 | gum choose | gh notifs open"
Sleep 500ms
Enter
Sleep 2s
Down
Sleep 1s
Enter
Sleep 2.5s

# Watch on the full screen.
Type "gh notifs watch"
Sleep 500ms
Enter
Sleep 3.5s
Type "q"
Sleep 1.5s
