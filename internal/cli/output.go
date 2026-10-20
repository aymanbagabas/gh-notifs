package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
	"github.com/cli/go-gh/v2/pkg/term"
	"github.com/cli/go-gh/v2/pkg/text"
	"github.com/muesli/termenv"
	xterm "golang.org/x/term"
)

const Columns = "id\tunread\tupdated_at\tlast_read_at\trepository\ttype\treason\treference\tstate\tauthor\ttitle\turl"

// WriteList uses a compact table for terminals and stable TSV for other writers.
func WriteList(ctx context.Context, w, stderr io.Writer, items []notifs.Thread, header, noColor bool) error {
	if file, ok := w.(*os.File); ok && term.IsTerminal(file) {
		width, _, err := xterm.GetSize(int(file.Fd()))
		if err != nil {
			return fmt.Errorf("get terminal size: %w", err)
		}
		var rendered bytes.Buffer
		if err := writeTable(&rendered, items, width, !noColor && !term.IsColorDisabled(), time.Now()); err != nil {
			return err
		}
		command := ""
		if len(items) > 0 {
			command, err = pagerCommand()
			if err != nil {
				return err
			}
		}
		return writePaged(ctx, w, stderr, command, rendered.Bytes())
	}
	if header {
		if _, err := fmt.Fprintln(w, Columns); err != nil {
			return err
		}
	}
	return WriteTSV(w, items)
}

func writeTable(w io.Writer, items []notifs.Thread, width int, color bool, now time.Time) error {
	if len(items) == 0 {
		_, err := fmt.Fprintln(w, "No notifications match your query.")
		return err
	}
	profile := termenv.Ascii
	if color {
		profile = termenv.ANSI
	}
	table := tableprinter.New(w, true, width)
	table.AddHeader([]string{"READ", "REFERENCE", "TITLE", "STATE", "REASON", "UPDATED"},
		tableprinter.WithColor(profile.String().Bold().Styled))
	for _, t := range items {
		read := "yes"
		readStyle := profile.String()
		if t.Unread {
			read = "no"
			readStyle = readStyle.Bold().Foreground(termenv.ANSICyan)
		}
		stateStyle := profile.String()
		switch t.State {
		case "open":
			stateStyle = stateStyle.Foreground(termenv.ANSIGreen)
		case "closed":
			stateStyle = stateStyle.Foreground(termenv.ANSIRed)
		case "merged":
			stateStyle = stateStyle.Foreground(termenv.ANSIMagenta)
		case "draft":
			stateStyle = stateStyle.Faint()
		}
		updated := ""
		if !t.UpdatedAt.IsZero() {
			updated = text.RelativeTimeAgo(now, t.UpdatedAt)
		}
		table.AddField(read, tableprinter.WithColor(readStyle.Styled))
		table.AddField(notifs.Plain(t.Reference()))
		table.AddField(notifs.Plain(t.Title))
		table.AddField(notifs.Plain(t.State), tableprinter.WithColor(stateStyle.Styled))
		table.AddField(notifs.Plain(notifs.ReasonName(t.Reason)))
		table.AddField(updated)
		table.EndRow()
	}
	return table.Render()
}

func WriteTSV(w io.Writer, items []notifs.Thread) error {
	// tableprinter's TSV methods discard write errors; Flush preserves them.
	out := bufio.NewWriter(w)
	table := tableprinter.New(out, false, 0)
	for _, t := range items {
		fields := []string{
			t.ID, strconv.FormatBool(t.Unread), timestamp(t.UpdatedAt), timestamp(t.LastReadAt),
			t.Repository, t.Type, t.Reason, t.Reference(), t.State, t.Author, t.Title, t.URL,
		}
		for _, field := range fields {
			table.AddField(notifs.Plain(field))
		}
		table.EndRow()
	}
	if err := table.Render(); err != nil {
		return err
	}
	return out.Flush()
}

func timestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
