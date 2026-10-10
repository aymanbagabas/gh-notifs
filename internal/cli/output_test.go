package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
)

func TestTSV(t *testing.T) {
	var out bytes.Buffer
	err := WriteTSV(&out, []notifs.Thread{{ID: "42", Repository: "acme/app", Number: 7,
		Type: "PullRequest", Reason: "review_requested",
		Title: "Hello\t世界\n\x1b[31m", Unread: true, URL: "https://github.com/acme/app/pull/7"}})
	if err != nil {
		t.Fatal(err)
	}

	fields := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\t")
	if len(fields) != 12 || fields[0] != "42" || fields[1] != "true" ||
		fields[5] != "PullRequest" || fields[6] != "review_requested" ||
		fields[7] != "acme/app#7" || fields[10] != "Hello 世界  [31m" {
		t.Fatalf("unexpected TSV: %q", out.String())
	}
	if strings.Count(out.String(), "\n") != 1 || strings.Contains(out.String(), "\x1b") {
		t.Fatalf("unsafe TSV: %q", out.String())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestTSVPreservesWriteErrors(t *testing.T) {
	for _, title := range []string{"short", strings.Repeat("large", 2000)} {
		err := WriteTSV(failingWriter{}, []notifs.Thread{{ID: "1", Title: title}})
		if !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("write error lost: %v", err)
		}
	}
}
