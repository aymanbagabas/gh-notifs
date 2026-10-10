package tui

import (
	stdcolor "image/color"
	"io"
	"strings"
	"testing"

	"altscreen.io/vision/buffer"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"github.com/aymanbagabas/gh-notifs/internal/notifs"
	"github.com/aymanbagabas/gh-notifs/internal/theme"
)

func assertButtonMnemonic(t *testing.T, sc *screen.Screen, target hit, label string, shortcut rune) {
	t.Helper()
	index := strings.IndexRune(label, shortcut)
	if index < 0 || target.rect.Dx() != len(label) {
		t.Fatalf("invalid button label or target: %q, %v", label, target.rect)
	}
	for x, r := range label {
		cell := sc.CellAt(layout.Pos(target.rect.Min.X+x, target.rect.Min.Y))
		if cell.Content() != string(r) || cell.Style.GetUnderline() != (x == index) {
			t.Fatalf("button %q at %d: content=%q, underline=%v", label, x, cell.Content(), cell.Style.GetUnderline())
		}
	}
}

func TestButtonMnemonics(t *testing.T) {
	for _, light := range []bool{false, true} {
		for _, noColor := range []bool{false, true} {
			a := testApp()
			a.light, a.options.NoColor, a.showPreview = light, noColor, true
			sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 50})
			a.paint(sc)
			for _, button := range []struct {
				action, label string
				shortcut      rune
			}{
				{"toggle", "Sidebar", 'b'},
				{"metadata-toggle", "hide metadata", 'm'},
				{"q", "quit", 'q'},
				{"x", "checkbox", 'x'},
				{"o", "open", 'o'},
				{"v", "preview", 'v'},
				{"r", "refresh", 'r'},
				{"e", "done", 'e'},
				{"shift+i", "I read it", 'I'},
				{"shift+m", "Mute", 'M'},
				{"shift+s", "Subscribe", 'S'},
			} {
				assertButtonMnemonic(t, sc, findHit(t, a, button.action), button.label, button.shortcut)
			}
			a.metadataHidden = true
			a.paint(sc)
			assertButtonMnemonic(t, sc, findHit(t, a, "metadata-toggle"), "metadata", 'm')
			a.confirmAction("done")
			for _, focused := range []bool{false, true} {
				for _, hover := range []int{0, 1, 2} {
					a.dialogConfirm, a.dialogHover = focused, hover
					a.paint(sc)
					assertButtonMnemonic(t, sc, findHit(t, a, "confirm"), " Yes ", 'Y')
					for _, target := range a.hits {
						if target.kind == "cancel" && target.index == 2 {
							assertButtonMnemonic(t, sc, target, " No ", 'N')
						}
					}
				}
			}
		}
	}
}

func TestPreviewAndPaneColors(t *testing.T) {
	closeColor := func(actual, expected stdcolor.Color) bool {
		ar, ag, ab, _ := actual.RGBA()
		er, eg, eb, _ := expected.RGBA()
		// Glamour's color conversion can round a channel down by one.
		return max(ar, er)-min(ar, er) <= 257 &&
			max(ag, eg)-min(ag, eg) <= 257 && max(ab, eb)-min(ab, eb) <= 257
	}
	for _, light := range []bool{false, true} {
		a := testApp()
		a.light, a.showPreview = light, true
		a.preview = &notifs.Preview{
			Title: "Vivid preview", Body: "Use `context.Context` with *care*.",
			Metadata: []notifs.PreviewSection{{Name: "Author", Items: []string{"@octocat"}, Total: -1}},
		}
		sc := screen.New(io.Discard, layout.Size{Width: 120, Height: 50})
		a.paint(sc)
		p := theme.Colors(light)
		find := func(token string) *buffer.Cell {
			t.Helper()
			for y := 27; y < 46; y++ {
				var line strings.Builder
				for x := 25; x < 120; x++ {
					line.WriteString(sc.CellAt(layout.Pos(x, y)).Content())
				}
				if x := strings.Index(line.String(), token); x >= 0 {
					return sc.CellAt(layout.Pos(25+x, y))
				}
			}
			t.Fatalf("preview does not contain %q", token)
			return nil
		}
		title := find("Vivid")
		if !closeColor(title.Style.GetForeground(), p.Accent) || !closeColor(title.Style.GetBackground(), p.Selection) {
			t.Fatalf("preview title lacks its accent band: light=%v foreground=%v background=%v; want %v / %v",
				light, title.Style.GetForeground(), title.Style.GetBackground(), p.Accent, p.Selection)
		}
		if !closeColor(find("Author").Style.GetForeground(), p.Cyan) {
			t.Fatal("metadata label lacks its cyan accent")
		}
		code := find("context.Context")
		if !closeColor(code.Style.GetForeground(), p.Purple) || !closeColor(code.Style.GetBackground(), p.Surface) ||
			!closeColor(find("care").Style.GetForeground(), p.Purple) {
			t.Fatal("inline code or emphasis lacks its distinct style")
		}
		for _, position := range []layout.Position{layout.Pos(0, 0), layout.Pos(1, 0), layout.Pos(0, 5), layout.Pos(1, 5), layout.Pos(1, 6), layout.Pos(50, 2)} {
			if sc.CellAt(position).Style.GetBackground() != p.Background {
				t.Fatalf("header, search, or sidebar has a tinted background at %v", position)
			}
		}
		if sc.CellAt(layout.Pos(26, 26)).Style.GetBackground() != p.Surface {
			t.Fatal("preview header lost its surface background")
		}
		a.edit = true
		a.paint(sc)
		if sc.CellAt(layout.Pos(0, 1)).Style.GetForeground() != p.Accent {
			t.Fatal("active search lacks an accent border")
		}
		if sc.CellAt(layout.Pos(50, 2)).Style.GetBackground() != p.Background {
			t.Fatal("active search has a tinted background")
		}
	}
}
