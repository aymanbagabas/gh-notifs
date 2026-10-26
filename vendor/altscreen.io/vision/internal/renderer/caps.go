package renderer

import (
	"strings"

	"altscreen.io/vision/term"
)

// Optimizations is the renderer capability bitset.
type Optimizations uint32

const (
	OptECH Optimizations = 1 << iota
	OptREP
	OptICH
	OptDCH
	OptCSR
	OptSUSD
	OptILDL
	OptBCE
	OptCHA
	OptHPA
	OptVPA
	OptTABS
	OptCBT
	OptCHT
	OptBS
	OptONLCR
)

const (
	allOptimizations = OptECH | OptREP | OptICH | OptDCH | OptCSR | OptSUSD | OptILDL | OptBCE | OptCHA | OptHPA | OptVPA | OptTABS | OptCBT | OptCHT | OptBS | OptONLCR
	lineDiscipline   = OptTABS | OptBS | OptONLCR
)

func NoOptimizations() Optimizations     { return 0 }
func ModernOptimizations() Optimizations { return allOptimizations &^ lineDiscipline }
func XtermOptimizations() Optimizations  { return ModernOptimizations() &^ (OptHPA | OptCHT | OptREP) }
func VT100Optimizations() Optimizations  { return OptICH | OptDCH | OptCSR | OptILDL }
func LinuxOptimizations() Optimizations  { return OptECH | OptICH | OptCHA | OptHPA | OptVPA }
func ScreenOptimizations() Optimizations {
	return OptICH | OptDCH | OptCSR | OptSUSD | OptILDL | OptHPA | OptVPA | OptCBT
}
func DefaultOptimizations() Optimizations { return XtermOptimizations() }

func (o Optimizations) Contains(flags Optimizations) bool       { return o&flags == flags }
func (o Optimizations) Intersects(flags Optimizations) bool     { return o&flags != 0 }
func (o Optimizations) Set(flags Optimizations) Optimizations   { return o | flags }
func (o Optimizations) Clear(flags Optimizations) Optimizations { return o &^ flags }

func (o Optimizations) with(flag Optimizations, enabled bool) Optimizations {
	if enabled {
		return o.Set(flag)
	}
	return o.Clear(flag)
}
func (o Optimizations) WithTabs(v bool) Optimizations  { return o.with(OptTABS, v) }
func (o Optimizations) WithBS(v bool) Optimizations    { return o.with(OptBS, v) }
func (o Optimizations) WithONLCR(v bool) Optimizations { return o.with(OptONLCR, v) }
func (o Optimizations) WithECH(v bool) Optimizations   { return o.with(OptECH, v) }
func (o Optimizations) WithREP(v bool) Optimizations   { return o.with(OptREP, v) }
func (o Optimizations) WithICH(v bool) Optimizations   { return o.with(OptICH, v) }
func (o Optimizations) WithDCH(v bool) Optimizations   { return o.with(OptDCH, v) }
func (o Optimizations) WithCSR(v bool) Optimizations   { return o.with(OptCSR, v) }
func (o Optimizations) WithSUSD(v bool) Optimizations  { return o.with(OptSUSD, v) }
func (o Optimizations) WithILDL(v bool) Optimizations  { return o.with(OptILDL, v) }
func (o Optimizations) WithBCE(v bool) Optimizations   { return o.with(OptBCE, v) }
func (o Optimizations) WithCHA(v bool) Optimizations   { return o.with(OptCHA, v) }
func (o Optimizations) WithHPA(v bool) Optimizations   { return o.with(OptHPA, v) }
func (o Optimizations) WithVPA(v bool) Optimizations   { return o.with(OptVPA, v) }
func (o Optimizations) WithCBT(v bool) Optimizations   { return o.with(OptCBT, v) }
func (o Optimizations) WithCHT(v bool) Optimizations   { return o.with(OptCHT, v) }

func OptimizationsFromTerm(name string) Optimizations {
	head := strings.SplitN(name, "-", 2)[0]
	if head == "xterm" && strings.HasPrefix(name, "xterm-") {
		vendor := strings.SplitN(strings.TrimPrefix(name, "xterm-"), "-", 2)[0]
		if vendor == "ghostty" || vendor == "kitty" || vendor == "rio" {
			return ModernOptimizations()
		}
	}
	switch head {
	case "", "dumb":
		return NoOptimizations()
	case "alacritty", "contour", "foot", "ghostty", "kitty", "rio", "st", "tmux", "wezterm":
		return ModernOptimizations()
	case "xterm":
		return XtermOptimizations()
	case "screen":
		return ScreenOptimizations()
	case "linux":
		return LinuxOptimizations()
	default:
		return NoOptimizations()
	}
}

func OptimizationsFromEnv(env term.Environ) Optimizations {
	if v, ok := env.LookupEnv("TERM"); ok {
		return OptimizationsFromTerm(v)
	}
	return DefaultOptimizations()
}

func supportsDECST8C(env term.Environ) bool {
	if hasEnv(env, "WT_SESSION") || hasEnv(env, "KITTY_WINDOW_ID") || hasEnv(env, "ALACRITTY_WINDOW_ID") || hasEnv(env, "ITERM_SESSION_ID") {
		return true
	}
	if v, ok := env.LookupEnv("LC_TERMINAL"); ok && strings.EqualFold(v, "iterm2") {
		return true
	}
	if v, ok := env.LookupEnv("TERM_PROGRAM"); ok {
		switch strings.ToLower(v) {
		case "ghostty", "rio", "iterm.app":
			return true
		}
	}
	termName, ok := env.LookupEnv("TERM")
	if !ok {
		return false
	}
	head := strings.SplitN(termName, "-", 2)[0]
	if head == "alacritty" || head == "ghostty" || head == "kitty" || head == "rio" {
		return true
	}
	if strings.HasPrefix(termName, "xterm-") {
		vendor := strings.SplitN(strings.TrimPrefix(termName, "xterm-"), "-", 2)[0]
		return vendor == "ghostty" || vendor == "kitty" || vendor == "rio"
	}
	return false
}

func hasEnv(env term.Environ, key string) bool { _, ok := env.LookupEnv(key); return ok }
