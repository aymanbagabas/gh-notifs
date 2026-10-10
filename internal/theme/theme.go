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
	Success    stdcolor.RGBA
	Danger     stdcolor.RGBA
	Warning    stdcolor.RGBA
	Purple     stdcolor.RGBA
}

func rgb(hex uint32) stdcolor.RGBA {
	return stdcolor.RGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}
}

func Colors(light bool) Palette {
	if light {
		return Palette{
			Background: rgb(0xffffff), Surface: rgb(0xf6f8fa), Foreground: rgb(0x1f2328),
			Muted: rgb(0x59636e), Border: rgb(0xd1d9e0), Accent: rgb(0x0969da),
			Selection: rgb(0xddf4ff), Success: rgb(0x1a7f37), Danger: rgb(0xcf222e),
			Warning: rgb(0x9a6700), Purple: rgb(0x8250df),
		}
	}
	return Palette{
		Background: rgb(0x0d1117), Surface: rgb(0x161b22), Foreground: rgb(0xe6edf3),
		Muted: rgb(0x9198a1), Border: rgb(0x3d444d), Accent: rgb(0x58a6ff),
		Selection: rgb(0x163356), Success: rgb(0x3fb950), Danger: rgb(0xf85149),
		Warning: rgb(0xd29922), Purple: rgb(0xbc8cff),
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
	muted, accent, green := color.Hex(p.Muted), color.Hex(p.Accent), color.Hex(p.Success)
	red, purple, border := color.Hex(p.Danger), color.Hex(p.Purple), color.Hex(p.Border)
	cfg.Document.Color, cfg.Document.BackgroundColor = &fg, &bg
	cfg.Heading.Color, cfg.H1.Color = &fg, &fg
	cfg.H6.Color = &fg
	cfg.H1.BackgroundColor = &bg
	cfg.BlockQuote.Color = &muted
	cfg.HorizontalRule.Color = &border
	cfg.Link.Color, cfg.LinkText.Color = &accent, &accent
	cfg.Image.Color, cfg.ImageText.Color = &accent, &muted
	cfg.Code.Color, cfg.Code.BackgroundColor = &fg, &surface
	cfg.CodeBlock.Color, cfg.CodeBlock.BackgroundColor = &fg, &surface
	cfg.CodeBlock.Theme = ""
	cfg.CodeBlock.Chroma = &glamouransi.Chroma{
		Text:                glamouransi.StylePrimitive{Color: &fg},
		Background:          glamouransi.StylePrimitive{BackgroundColor: &surface},
		Error:               glamouransi.StylePrimitive{Color: &red},
		Comment:             glamouransi.StylePrimitive{Color: &muted},
		CommentPreproc:      glamouransi.StylePrimitive{Color: &muted},
		Keyword:             glamouransi.StylePrimitive{Color: &red},
		KeywordType:         glamouransi.StylePrimitive{Color: &red},
		KeywordNamespace:    glamouransi.StylePrimitive{Color: &red},
		Operator:            glamouransi.StylePrimitive{Color: &accent},
		Punctuation:         glamouransi.StylePrimitive{Color: &fg},
		Name:                glamouransi.StylePrimitive{Color: &fg},
		NameBuiltin:         glamouransi.StylePrimitive{Color: &purple},
		NameFunction:        glamouransi.StylePrimitive{Color: &purple},
		NameClass:           glamouransi.StylePrimitive{Color: &purple},
		NameTag:             glamouransi.StylePrimitive{Color: &green},
		NameAttribute:       glamouransi.StylePrimitive{Color: &accent},
		NameConstant:        glamouransi.StylePrimitive{Color: &accent},
		LiteralString:       glamouransi.StylePrimitive{Color: &accent},
		LiteralStringEscape: glamouransi.StylePrimitive{Color: &green},
		LiteralNumber:       glamouransi.StylePrimitive{Color: &accent},
		GenericDeleted:      glamouransi.StylePrimitive{Color: &red},
		GenericInserted:     glamouransi.StylePrimitive{Color: &green},
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
	return markdown.Render(source, glamour.WithStyles(cfg), glamour.WithColorProfile(profile),
		markdown.WithWrap(width), markdown.WithBaseURL(baseURL))
}
