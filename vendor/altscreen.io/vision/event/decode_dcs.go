package event

import "altscreen.io/vision/ansi"

func (d *decoder) parseDCS(buf []byte) parseResult {
	p := introPrefixLen(buf[0])
	end, st, ok := findStringTerminator(buf[p:])
	if !ok {
		return resultIncomplete()
	}
	payload := buf[p : p+end]
	return resultEvent(finalizeDCS(payload), p+end+st)
}

func finalizeDCS(payload []byte) Event {
	// The regions a DCS carries in front of its payload are the ones a CSI
	// body has, so the same parse reads both. The tail after the final byte is
	// the payload here, where a CSI has none.
	cs, data, ok := ansi.ParseControlSequence(payload)
	if !ok {
		return UnknownDcsEvent(append([]byte(nil), payload...))
	}
	priv, final := cs.Private(), cs.Final()
	params, inter := cs.Params().Raw(), cs.Intermediate()
	if inter == '+' && final == 'r' && (string(params) == "1" || string(params) == "0") {
		return TermcapEvent{Recognized: string(params) == "1", Entries: decodeTermcapPayload(data)}
	}
	if inter == '$' && final == 'r' && (string(params) == "1" || string(params) == "0") {
		if string(params) == "1" {
			return SettingReportEvent(recognizeSetting(data))
		}
		return SettingReportEvent(SettingReport{})
	}
	if priv == '>' && final == '|' {
		return TerminalNameEvent(string(data))
	}
	if inter == '!' && final == '|' {
		if dec, ok := hexDecode(data); ok {
			return TertiaryDeviceAttributesEvent(string(dec))
		}
		return TertiaryDeviceAttributesEvent(string(data))
	}
	return UnknownDcsEvent(append([]byte(nil), payload...))
}

// recognizeSetting decodes the setting a valid DECRPSS reply carries.
//
// The reply spells the control function out the way a CSI would, without the
// introducer, so "2 q" is the terminal reporting a steady block for DECSCUSR.
// Anything this does not recognize is handed back verbatim in Raw, which is
// also always set so a caller that knows the function can read it either way.
func recognizeSetting(data []byte) SettingReport {
	report := SettingReport{Raw: string(data), Recognized: true}
	cs, rest, ok := ansi.ParseControlSequence(data)
	// A setting is the whole reply, so anything left over means this was read
	// as something it is not.
	if !ok || len(rest) > 0 {
		return report
	}
	// DECSCUSR: CSI Ps SP q. A reply states the current setting, so an omitted
	// parameter is not expected here and reads as 0, the terminal default.
	// Terminals do not agree on that reading when *setting* a style: kitty and
	// Konsole take a bare CSI SP q as a blinking block, while foot, VTE and
	// Windows Terminal take it as 0.
	if cs.Private() == 0 && cs.Intermediate() == ' ' && cs.Final() == 'q' {
		if style, ok := ansi.CursorStyleFromParam(cs.Params().GetOr(0, 0)); ok {
			report.Setting = CursorStyleSetting{Style: style}
		}
	}
	return report
}
