package theme

import (
	stdcolor "image/color"
	"strings"
	"unicode"

	"altscreen.io/vision/color"
	"github.com/charmbracelet/glamour"
	glamouransi "github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/cli/go-gh/v2/pkg/markdown"
	"github.com/muesli/termenv"
)

type Palette struct {
	Background stdcolor.RGBA
	Surface    stdcolor.RGBA
	Foreground stdcolor.RGBA
	Muted      stdcolor.RGBA
	Border     stdcolor.RGBA
	Accent     stdcolor.RGBA
	Selection  stdcolor.RGBA
	Selected   stdcolor.RGBA
	Success    stdcolor.RGBA
	Danger     stdcolor.RGBA
	Warning    stdcolor.RGBA
	Purple     stdcolor.RGBA
	Cyan       stdcolor.RGBA
}

func rgb(hex uint32) stdcolor.RGBA {
	return stdcolor.RGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}
}

func Colors(light bool) Palette {
	if light {
		return Palette{
			Background: rgb(0xffffff), Surface: rgb(0xf6f8fa), Foreground: rgb(0x1f2328),
			Muted: rgb(0x59636e), Border: rgb(0xd1d9e0), Accent: rgb(0x0969da),
			Selection: rgb(0xddf4ff), Selected: rgb(0xeaf5ff), Success: rgb(0x1a7f37), Danger: rgb(0xcf222e),
			Warning: rgb(0x9a6700), Purple: rgb(0x8250df), Cyan: rgb(0x0e7490),
		}
	}
	return Palette{
		Background: rgb(0x0d1117), Surface: rgb(0x161b22), Foreground: rgb(0xe6edf3),
		Muted: rgb(0x9198a1), Border: rgb(0x3d444d), Accent: rgb(0x58a6ff),
		Selection: rgb(0x163356), Selected: rgb(0x101f31), Success: rgb(0x3fb950), Danger: rgb(0xf85149),
		Warning: rgb(0xd29922), Purple: rgb(0xbc8cff), Cyan: rgb(0x76d8d8),
	}
}

func IsLight(background stdcolor.Color) bool {
	r, g, b, _ := background.RGBA()
	return 0.299*float64(r)+0.587*float64(g)+0.114*float64(b) >= 0.5*65535
}

func MarkdownStyle(light bool) glamouransi.StyleConfig {
	p := Colors(light)
	cfg := styles.DarkStyleConfig
	if light {
		cfg = styles.LightStyleConfig
	}
	fg, bg, surface := color.Hex(p.Foreground), color.Hex(p.Background), color.Hex(p.Surface)
	muted, accent := color.Hex(p.Muted), color.Hex(p.Accent)
	purple, border := color.Hex(p.Purple), color.Hex(p.Border)
	cyan, selection := color.Hex(p.Cyan), color.Hex(p.Selection)
	enabled := true
	cfg.Document.Color, cfg.Document.BackgroundColor = &fg, &bg
	cfg.Heading.Color, cfg.H1.Color = &cyan, &accent
	cfg.H1.BackgroundColor = &selection
	cfg.H2.Color, cfg.H3.Color = &cyan, &purple
	cfg.H4.Color, cfg.H5.Color, cfg.H6.Color = &fg, &muted, &muted
	cfg.Strong.Color, cfg.Emph.Color = &cyan, &purple
	cfg.Strong.Bold, cfg.Emph.Italic = &enabled, &enabled
	cfg.BlockQuote.Color = &muted
	cfg.BlockQuote.Italic = &enabled
	cfg.BlockQuote.BackgroundColor = &surface
	cfg.HorizontalRule.Color = &border
	cfg.Link.Color, cfg.LinkText.Color = &accent, &accent
	cfg.Image.Color, cfg.ImageText.Color = &accent, &muted
	cfg.Code.Color, cfg.Code.BackgroundColor = &purple, &surface
	cfg.Table.BackgroundColor = &surface
	center, column, row := "┼", "│", "─"
	cfg.Table.CenterSeparator, cfg.Table.ColumnSeparator, cfg.Table.RowSeparator = &center, &column, &row
	cfg.DefinitionTerm.Color, cfg.DefinitionTerm.Bold = &cyan, &enabled
	cfg.Strikethrough.Color = &muted
	cfg.CodeBlock.Color, cfg.CodeBlock.BackgroundColor = &fg, &surface
	// Glamour registers custom Chroma styles only once, so use separate named themes.
	cfg.CodeBlock.Chroma = nil
	cfg.CodeBlock.Theme = "github-dark"
	if light {
		cfg.CodeBlock.Theme = "github"
	}
	return cfg
}

func Render(source, baseURL string, width int, light, noColor bool) (string, error) {
	source = strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf)) {
			return ' '
		}
		return r
	}, source)
	cfg, profile := MarkdownStyle(light), termenv.TrueColor
	if noColor {
		cfg, profile = styles.ASCIIStyleConfig, termenv.Ascii
	}
	// JSON style overrides reuse pointers from Glamour's shared defaults.
	cfg.Document.Margin, cfg.CodeBlock.Margin = new(uint), new(uint)
	return markdown.Render(source, glamour.WithStyles(cfg), glamour.WithColorProfile(profile), glamour.WithChromaFormatter("terminal16m"),
		markdown.WithWrap(width), markdown.WithBaseURL(baseURL))
}
