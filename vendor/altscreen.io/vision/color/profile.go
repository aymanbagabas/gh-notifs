package color

import (
	"image/color"
	"runtime"
	"strconv"
	"strings"

	"altscreen.io/vision/term"
)

// Profile is a terminal color capability profile.
type Profile uint8

const (
	// Disabled emits no styling output.
	Disabled Profile = iota
	// Ascii emits text decoration but no color.
	Ascii
	// Ansi emits the standard 16-color ANSI palette.
	Ansi
	// Ansi256 emits the xterm 256-color palette.
	Ansi256
	// TrueColor emits 24-bit true color.
	TrueColor
)

// Default returns the default color profile.
func Default() Profile { return TrueColor }

// Convert downsample c to fit p, or nil when p strips color.
func (p Profile) Convert(c color.Color) color.Color {
	if c == nil {
		return nil
	}
	switch p {
	case Disabled, Ascii:
		return nil
	case Ansi:
		return rgbToAnsi16(ToRGB(c))
	case Ansi256:
		return rgbToAnsi256(ToRGB(c))
	case TrueColor:
		return c
	default:
		return c
	}
}

// Detect detects the color profile from the current process environment.
func Detect() Profile { return DetectFrom(term.ProcessEnv{}, true) }

// DetectFrom detects the color profile from env and explicit TTY state.
func DetectFrom(env term.Environ, isTTY bool) Profile {
	isTTY = isTTY || isTruthy(env, "TTY_FORCE")
	termName := env.Getenv("TERM")
	envp := envColorProfile(env, termName)
	p := envp
	if !isTTY {
		p = Disabled
	}
	if term.HasEnv(env, "NO_COLOR") && isTTY {
		if p > Ascii {
			p = Ascii
		}
		return p
	}
	if isTruthy(env, "CLICOLOR_FORCE") {
		if p < Ansi {
			p = Ansi
		}
		if envp > p {
			p = envp
		}
		return p
	}
	isDumb := termName == "" || termName == dumbTerm
	if isTruthy(env, "CLICOLOR") && isTTY && !isDumb && p < Ansi {
		p = Ansi
	}
	return p
}

// String returns the profile name.
func (p Profile) String() string {
	if int(p) < len(profileNames) {
		return profileNames[p]
	}
	return "Profile(" + strconv.Itoa(int(p)) + ")"
}

var profileNames = [...]string{"Disabled", "Ascii", "Ansi", "Ansi256", "TrueColor"}

const dumbTerm = "dumb"

func isTruthy(env term.Environ, key string) bool {
	v, err := strconv.ParseBool(env.Getenv(key))
	return err == nil && v
}

func envColorProfile(env term.Environ, termName string) Profile {
	p := Ansi
	switch {
	case termName == dumbTerm:
		p = Disabled
	case termName == "":
		if runtime.GOOS == "windows" {
			p = windowsColorProfile(env)
		} else {
			p = Disabled
		}
	}

	for _, t := range knownTrueColorTerms {
		if strings.Contains(termName, t) {
			return TrueColor
		}
	}
	if strings.HasPrefix(termName, "tmux") || strings.HasPrefix(termName, "screen") {
		if p < Ansi256 {
			p = Ansi256
		}
	} else if strings.HasPrefix(termName, "xterm") && p < Ansi {
		p = Ansi
	}
	if term.HasEnv(env, "WT_SESSION") || isTruthy(env, "GOOGLE_CLOUD_SHELL") || isTruthy(env, "CI") {
		return TrueColor
	}
	if colortermSaysTrueColor(env) && !strings.HasPrefix(termName, "screen") {
		return TrueColor
	}
	if strings.HasSuffix(termName, "256color") && p < Ansi256 {
		p = Ansi256
	}
	if strings.HasSuffix(termName, "direct") {
		return TrueColor
	}
	return p
}

var knownTrueColorTerms = [...]string{"alacritty", "contour", "foot", "ghostty", "kitty", "rio", "st", "wezterm"}

func colortermSaysTrueColor(env term.Environ) bool {
	switch strings.ToLower(env.Getenv("COLORTERM")) {
	case "truecolor", "24bit", "yes", "true":
		return true
	default:
		return false
	}
}

func windowsColorProfile(env term.Environ) Profile {
	if term.HasEnv(env, "WT_SESSION") {
		return TrueColor
	}
	return Ansi256
}
