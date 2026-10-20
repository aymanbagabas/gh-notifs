package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cli/go-gh/v2/pkg/config"
)

func unsetTestEnv(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

func TestPagerCommand(t *testing.T) {
	original := config.Read
	t.Cleanup(func() { config.Read = original })
	for _, tc := range []struct {
		name, ghPager, setting, pager, want string
		ghSet, badConfig                    bool
	}{
		{name: "GH_PAGER wins", ghSet: true, ghPager: "less -R", setting: "more", pager: "other", want: "less -R"},
		{name: "empty GH_PAGER disables", ghSet: true, setting: "more", pager: "other"},
		{name: "cat disables", ghSet: true, ghPager: "cat", badConfig: true, want: "cat"},
		{name: "config wins", setting: "less", pager: "other", want: "less"},
		{name: "PAGER fallback", pager: "more", want: "more"},
		{name: "unconfigured"},
		{name: "config error", badConfig: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unsetTestEnv(t, "GH_PAGER")
			if tc.ghSet {
				t.Setenv("GH_PAGER", tc.ghPager)
			}
			t.Setenv("PAGER", tc.pager)
			config.Read = func(*config.Config) (*config.Config, error) {
				if tc.badConfig {
					return nil, errors.New("invalid config")
				}
				cfg := config.ReadFromString("")
				if tc.setting != "" {
					cfg.Set([]string{"pager"}, tc.setting)
				}
				return cfg, nil
			}
			got, err := pagerCommand()
			if tc.badConfig && !tc.ghSet {
				if err == nil || !strings.Contains(err.Error(), "invalid config") {
					t.Fatalf("expected configuration error, got %v", err)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func pagerTestCommand() string {
	return fmt.Sprintf("%q -test.run=^TestPagerProcess$ -- \"two words\"", os.Args[0])
}

func TestPagerProcess(t *testing.T) {
	switch os.Getenv("GH_NOTIFS_TEST_PAGER") {
	case "copy":
		if _, err := io.Copy(os.Stdout, os.Stdin); err != nil {
			os.Exit(1)
		}
	case "environment":
		if err := json.NewEncoder(os.Stdout).Encode(map[string]string{
			"LESS": os.Getenv("LESS"), "LV": os.Getenv("LV"), "PAGER": os.Getenv("PAGER"),
			"argument": os.Args[len(os.Args)-1],
		}); err != nil {
			os.Exit(1)
		}
	case "early":
	case "fail":
		fmt.Fprintln(os.Stderr, "pager failed")
		os.Exit(7)
	case "wait":
		fmt.Fprintln(os.Stderr, "ready")
		time.Sleep(time.Hour)
	default:
		return
	}
	os.Exit(0)
}

func TestWritePaged(t *testing.T) {
	t.Setenv("GH_NOTIFS_TEST_PAGER", "copy")
	content := []byte("READ  TITLE\nno    Hello 世界\n")
	for _, command := range []string{"", "cat", pagerTestCommand()} {
		var out, stderr bytes.Buffer
		if err := writePaged(context.Background(), &out, &stderr, command, content); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out.Bytes(), content) || stderr.Len() != 0 {
			t.Fatalf("command %q: output=%q stderr=%q", command, out.String(), stderr.String())
		}
		if err := writePaged(context.Background(), failingWriter{}, &stderr, command, content); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("write error lost for %q: %v", command, err)
		}
	}
}

func TestPagerEnvironment(t *testing.T) {
	t.Setenv("GH_NOTIFS_TEST_PAGER", "environment")
	t.Setenv("PAGER", "must not reach the child")
	for _, tc := range []struct {
		name, less, lv string
		set            bool
	}{
		{name: "defaults", less: "FRX", lv: "-c"},
		{name: "custom", less: "-R", lv: "-a", set: true},
		{name: "explicit empty", set: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unsetTestEnv(t, "LESS")
			unsetTestEnv(t, "LV")
			if tc.set {
				t.Setenv("LESS", tc.less)
				t.Setenv("LV", tc.lv)
			}
			var out, stderr bytes.Buffer
			if err := writePaged(context.Background(), &out, &stderr, pagerTestCommand(), nil); err != nil {
				t.Fatal(err)
			}
			var got map[string]string
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got["LESS"] != tc.less || got["LV"] != tc.lv || got["PAGER"] != "" || got["argument"] != "two words" {
				t.Fatalf("unexpected child environment or arguments: %v", got)
			}
			if os.Getenv("PAGER") != "must not reach the child" {
				t.Fatal("pager changed the parent environment")
			}
			if _, set := os.LookupEnv("LESS"); set != tc.set {
				t.Fatal("pager changed the parent LESS setting")
			}
		})
	}
}

func TestPagerStartFailurePrintsWarningAndTable(t *testing.T) {
	for _, command := range []string{"not-a-real-gh-notifs-test-pager", `"unterminated`, "   "} {
		var out, stderr bytes.Buffer
		err := writePaged(context.Background(), &out, &stderr, command, []byte("table\n"))
		if err != nil || out.String() != "table\n" || !strings.Contains(stderr.String(), "failed to start pager:") {
			t.Fatalf("command %q: output=%q stderr=%q error=%v", command, out.String(), stderr.String(), err)
		}
	}
	err := writePaged(context.Background(), io.Discard, failingWriter{}, `"unterminated`, nil)
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("warning error lost: %v", err)
	}
}

func TestPagerEarlyExitAndFailure(t *testing.T) {
	for _, mode := range []string{"early", "fail"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("GH_NOTIFS_TEST_PAGER", mode)
			var stderr bytes.Buffer
			err := writePaged(context.Background(), io.Discard, &stderr, pagerTestCommand(), bytes.Repeat([]byte("row\n"), 100000))
			if mode == "early" && err != nil {
				t.Fatalf("early pager exit failed: %v", err)
			}
			if mode == "fail" && (err == nil || !strings.Contains(err.Error(), "exit status 7") || stderr.String() != "pager failed\n") {
				t.Fatalf("pager error lost: %v, %q", err, stderr.String())
			}
		})
	}
}

type cancelPagerWriter struct{ cancel context.CancelFunc }

func (w cancelPagerWriter) Write(p []byte) (int, error) {
	w.cancel()
	return len(p), nil
}

func TestPagerCancellation(t *testing.T) {
	t.Setenv("GH_NOTIFS_TEST_PAGER", "wait")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := writePaged(ctx, io.Discard, cancelPagerWriter{cancel}, pagerTestCommand(), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestPipedListBypassesPager(t *testing.T) {
	t.Setenv("GH_PAGER", `"unterminated`)
	var out, stderr bytes.Buffer
	if err := WriteList(context.Background(), &out, &stderr, nil, true, false); err != nil {
		t.Fatal(err)
	}
	if out.String() != Columns+"\n" || stderr.Len() != 0 {
		t.Fatalf("output=%q stderr=%q", out.String(), stderr.String())
	}
}
