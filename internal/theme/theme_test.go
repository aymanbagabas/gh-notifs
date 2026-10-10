package theme

import (
	"encoding/json"
	"image/color"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/glamour/styles"
)

func TestBackgroundDetection(t *testing.T) {
	if IsLight(color.Black) || !IsLight(color.White) || IsLight(Colors(false).Background) || !IsLight(Colors(true).Background) {
		t.Fatal("incorrect background classification")
	}
}

func TestMarkdownThemes(t *testing.T) {
	beforeDark, _ := json.Marshal(styles.DarkStyleConfig)
	beforeLight, _ := json.Marshal(styles.LightStyleConfig)
	for _, light := range []bool{false, true} {
		rendered, err := Render("# Heading\n\n**Bold** and [link](https://github.com).\n\n```go\nreturn true\n```\n", "", 60, light, false)
		if err != nil || !strings.Contains(rendered, "Heading") || !strings.Contains(rendered, "\x1b[") {
			t.Fatalf("styled preview failed: %v", err)
		}
		plain, err := Render("**Bold**\n\nText", "", 60, light, true)
		if err != nil || strings.Contains(plain, "\x1b") || !strings.Contains(plain, "Bold") {
			t.Fatalf("no-color preview failed: %q, %v", plain, err)
		}
	}
	afterDark, _ := json.Marshal(styles.DarkStyleConfig)
	afterLight, _ := json.Marshal(styles.LightStyleConfig)
	if string(beforeDark) != string(afterDark) || string(beforeLight) != string(afterLight) {
		t.Fatal("rendering mutated shared Glamour styles")
	}
}

func TestRemoteControlsAreNotExecuted(t *testing.T) {
	out, err := Render("hello \x1b]52;c;secret\a \x1b[2J world", "", 60, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\x1b]52") || strings.Contains(out, "\x1b[2J") {
		t.Fatal("untrusted terminal controls survived Markdown rendering")
	}
}

func TestExportedThemesMatchPalette(t *testing.T) {
	for _, name := range []string{"dark", "light"} {
		b, err := os.ReadFile("../../themes/github-" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var got, want any
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		expected, err := json.Marshal(MarkdownStyle(name == "light"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(expected, &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("regenerate %s theme with go run ./examples/theme --export themes", name)
		}
	}
}
