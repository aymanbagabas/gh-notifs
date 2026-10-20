package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/cli/go-gh/v2/pkg/text"
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

func TestListPreservesTSV(t *testing.T) {
	t.Setenv("GH_FORCE_TTY", "120")
	t.Setenv("CLICOLOR_FORCE", "1")
	items := []notifs.Thread{{
		ID: "42", Repository: "acme/app", Number: 7, Type: "PullRequest",
		Reason: "review_requested", Unread: true, State: "open", Author: "example",
		Title: "Fix slow uploads", URL: "https://github.com/acme/app/pull/7",
		UpdatedAt: time.Date(2026, 10, 19, 12, 0, 0, 0, time.FixedZone("offset", 3600)),
	}}
	want := "42\ttrue\t2026-10-19T11:00:00Z\t\tacme/app\tPullRequest\treview_requested\tacme/app#7\topen\texample\tFix slow uploads\thttps://github.com/acme/app/pull/7\n"
	for _, header := range []bool{false, true} {
		for _, noColor := range []bool{false, true} {
			var out bytes.Buffer
			if err := WriteList(context.Background(), &out, io.Discard, items, header, noColor); err != nil {
				t.Fatal(err)
			}
			expected := want
			if header {
				expected = Columns + "\n" + expected
			}
			if out.String() != expected {
				t.Fatalf("header=%t noColor=%t: got %q, want %q", header, noColor, out.String(), expected)
			}
			if err := WriteList(context.Background(), failingWriter{}, io.Discard, items, header, noColor); !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("write error lost: %v", err)
			}
		}
	}
	var out bytes.Buffer
	if err := WriteList(context.Background(), &out, io.Discard, nil, false, false); err != nil || out.Len() != 0 {
		t.Fatalf("empty TSV: %q, %v", out.String(), err)
	}
	file, err := os.CreateTemp(t.TempDir(), "notifications-*.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := WriteList(context.Background(), file, io.Discard, items, true, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil || string(data) != Columns+"\n"+want {
		t.Fatalf("redirected output: %q, %v", data, err)
	}
}

func TestTerminalTable(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	items := []notifs.Thread{
		{Repository: "acme/app", Number: 7, Title: "Fix slow uploads", Unread: true,
			State: "open", Reason: "review_requested", UpdatedAt: now.Add(-2 * time.Hour)},
		{Repository: "acme/app", Number: 8, Title: "Document workflow", State: "closed",
			Reason: "mention", UpdatedAt: now.Add(-24 * time.Hour)},
		{Repository: "acme/app", Title: "Release available", Reason: "subscribed"},
		{Repository: "acme/app", Number: 9, Title: "Merged change", State: "merged"},
		{Repository: "acme/app", Number: 10, Title: "Draft change", State: "draft"},
	}
	for _, color := range []bool{false, true} {
		var out bytes.Buffer
		if err := writeTable(&out, items, 120, color, now); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if strings.Contains(got, "\t") || strings.Contains(got, "\x1b[") != color {
			t.Fatalf("unexpected format with color=%t: %q", color, got)
		}
		for _, want := range []string{"READ", "REFERENCE", "TITLE", "STATE", "REASON", "UPDATED",
			"yes", "no", "acme/app#7", "Fix slow uploads", "review-requested", "about 2 hours ago",
			"about 1 day ago", "Release available", "open", "closed", "merged", "draft"} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in %q", want, got)
			}
		}
		if strings.Contains(got, "0001") {
			t.Fatalf("zero date rendered: %q", got)
		}
		if lines := strings.Count(got, "\n"); lines != len(items)+1 {
			t.Fatalf("got %d lines: %q", lines, got)
		}
	}
}

func TestTerminalTableFitsWidthAndSanitizesValues(t *testing.T) {
	item := notifs.Thread{
		Repository: "acme/\x1bapp", Number: 7,
		Title: strings.Repeat("Hello 世界\t\n\x1b[31m\u202e", 8),
		State: "open\n\x1b", Reason: "review_requested", UpdatedAt: time.Now(),
	}
	for _, width := range []int{20, 40, 80, 120} {
		var out bytes.Buffer
		if err := writeTable(&out, []notifs.Thread{item}, width, false, time.Now()); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if strings.ContainsAny(got, "\x1b\t\u202e") || strings.Count(got, "\n") != 2 {
			t.Fatalf("unsafe table: %q", got)
		}
		for _, line := range strings.Split(got, "\n") {
			if text.DisplayWidth(line) > width {
				t.Errorf("line exceeds width %d: %q", width, line)
			}
		}
	}
}

func TestTerminalTableEmptyAndWriteErrors(t *testing.T) {
	var out bytes.Buffer
	if err := writeTable(&out, nil, 80, false, time.Now()); err != nil ||
		out.String() != "No notifications match your query.\n" {
		t.Fatalf("empty table: %q, %v", out.String(), err)
	}
	for _, items := range [][]notifs.Thread{nil, {{Title: "Example"}}} {
		if err := writeTable(failingWriter{}, items, 80, true, time.Now()); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("write error lost: %v", err)
		}
	}
}
