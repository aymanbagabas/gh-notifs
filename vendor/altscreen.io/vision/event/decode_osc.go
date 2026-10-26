package event

import (
	"image/color"
	"strconv"
	"strings"

	"altscreen.io/vision/ansi"
)

func (d *decoder) parseOSC(buf []byte) parseResult {
	p := introPrefixLen(buf[0])
	for i := p; i < len(buf); i++ {
		if buf[i] == 0x07 || buf[i] == 0x9c {
			return resultEvent(finalizeOSC(buf[p:i]), i+1)
		}
		if buf[i] == 0x1b && i+1 < len(buf) && buf[i+1] == '\\' {
			return resultEvent(finalizeOSC(buf[p:i]), i+2)
		}
	}
	return resultIncomplete()
}

func finalizeOSC(payload []byte) Event {
	parts := strings.SplitN(string(payload), ";", 2)
	if len(parts) != 2 {
		return UnknownOscEvent(append([]byte(nil), payload...))
	}
	cmd, err := strconv.Atoi(parts[0])
	if err != nil {
		return UnknownOscEvent(append([]byte(nil), payload...))
	}
	rest := parts[1]
	switch cmd {
	case 7501:
		if strings.HasPrefix(rest, "?") {
			return parseProgramStatus(rest)
		}
	case 4:
		p := strings.SplitN(rest, ";", 2)
		if len(p) == 2 {
			if idx, err := strconv.Atoi(p[0]); err == nil {
				if c, ok := parseOSCColor(p[1]); ok {
					return PaletteColorEvent{Index: idx, Color: c}
				}
			}
		}
	case 10:
		if c, ok := parseOSCColor(rest); ok {
			return ForegroundColorEvent{Color: c}
		}
	case 11:
		if c, ok := parseOSCColor(rest); ok {
			return BackgroundColorEvent{Color: c}
		}
	case 12:
		if c, ok := parseOSCColor(rest); ok {
			return CursorColorEvent{Color: c}
		}
	case 52:
		p := strings.SplitN(rest, ";", 2)
		if len(p) == 2 {
			sel := ClipboardSelection{Kind: ClipboardSelectionSystem}
			if p[0] != "" {
				switch []rune(p[0])[0] {
				case 'c':
					sel.Kind = ClipboardSelectionSystem
				case 'p':
					sel.Kind = ClipboardSelectionPrimary
				default:
					sel.Kind = ClipboardSelectionOther
					sel.Other = []rune(p[0])[0]
				}
			}
			return ClipboardEvent{Selection: sel, Content: p[1]}
		}
	}
	return UnknownOscEvent(append([]byte(nil), payload...))
}

func parseProgramStatus(s string) ProgramStatusEvent {
	caps := ansi.ProgramStatusCapabilities{
		States: []ansi.ProgramStatusState{"idle", "working", "done", "blocked", "error"},
		Kinds:  []ansi.ProgramStatusKind{"permission", "question", "auth"},
	}
	for pair := range strings.SplitSeq(s, ":") {
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.Trim(value, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_.,+/=-") != "" {
			continue
		}
		switch strings.TrimSpace(key) {
		case "states":
			caps.States = []ansi.ProgramStatusState{}
			for name := range strings.SplitSeq(value, ",") {
				if name != "" {
					caps.States = append(caps.States, ansi.ProgramStatusState(name))
				}
			}
		case "kinds":
			caps.Kinds = []ansi.ProgramStatusKind{}
			for name := range strings.SplitSeq(value, ",") {
				if name != "" {
					caps.Kinds = append(caps.Kinds, ansi.ProgramStatusKind(name))
				}
			}
		}
	}
	return ProgramStatusEvent{Capabilities: caps}
}

func parseOSCColor(s string) (color.Color, bool) {
	s, ok := strings.CutPrefix(s, "rgb:")
	if !ok {
		return nil, false
	}
	p := strings.Split(s, "/")
	if len(p) != 3 {
		return nil, false
	}
	r, ok := parseHexChannel(p[0])
	if !ok {
		return nil, false
	}
	g, ok := parseHexChannel(p[1])
	if !ok {
		return nil, false
	}
	b, ok := parseHexChannel(p[2])
	if !ok {
		return nil, false
	}
	return color.RGBA{R: r, G: g, B: b, A: 0xff}, true
}

func parseHexChannel(s string) (uint8, bool) {
	if len(s) == 0 || len(s) > 4 {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return 0, false
	}
	switch len(s) {
	case 1:
		return uint8(v * 0x11), true
	case 2:
		return uint8(v), true
	case 3:
		return uint8(v >> 4), true
	default:
		return uint8(v >> 8), true
	}
}
