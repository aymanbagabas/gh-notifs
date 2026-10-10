package cli

import (
	"bufio"
	"io"
	"strconv"
	"time"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/cli/go-gh/v2/pkg/tableprinter"
)

const Columns = "id\tunread\tupdated_at\tlast_read_at\trepository\ttype\treason\treference\tstate\tauthor\ttitle\turl"

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
