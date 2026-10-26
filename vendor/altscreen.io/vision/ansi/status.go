package ansi

import "strconv"

// StatusRequestCursorPosition asks for a cursor position report: ESC [ 6 n.
const StatusRequestCursorPosition = RequestCursorPosition

// StatusRequestExtendedCursorPosition asks for an extended cursor position report: ESC [ ? 6 n.
const StatusRequestExtendedCursorPosition = RequestExtendedCursorPosition

// RequestLightDarkReport requests a light or dark preference report: ESC [ ? 996 n.
const RequestLightDarkReport = "\x1b[?996n"

// RequestVisibilityReport requests a terminal visibility report: ESC [ ? 998 n.
const RequestVisibilityReport = "\x1b[?998n"

// DECRQSS requests a current setting with DCS $ q selector ST.
func DECRQSS(selector string) string { return "\x1bP$q" + selector + "\x1b\\" }

// DSRRequest encodes a Device Status Report request.
func DSRRequest(dec bool, ps int) string {
	if dec {
		return "\x1b[?" + strconv.Itoa(ps) + "n"
	}
	return "\x1b[" + strconv.Itoa(ps) + "n"
}

// CPR encodes a standard cursor position report: ESC [ line ; column R.
func CPR(line, column int) string {
	return "\x1b[" + strconv.Itoa(max(line, 1)) + ";" + strconv.Itoa(max(column, 1)) + "R"
}

// DECXCPR encodes an extended cursor position report.
func DECXCPR(line, column, page int) string {
	prefix := "\x1b[?" + strconv.Itoa(max(line, 1)) + ";" + strconv.Itoa(max(column, 1))
	if page == 0 {
		return prefix + "R"
	}
	return prefix + ";" + strconv.Itoa(page) + "R"
}

// DECRPSS encodes a DECRPSS setting report with DCS ps $ r settings ST.
func DECRPSS(valid bool, settings string) string {
	if !valid {
		return "\x1bP0$r\x1b\\"
	}
	return "\x1bP1$r" + settings + "\x1b\\"
}

// LightDarkReport encodes a light or dark preference report response.
func LightDarkReport(dark bool) string {
	if dark {
		return "\x1b[?997;1n"
	}
	return "\x1b[?997;2n"
}

// VisibilityReport encodes a terminal visibility report response.
func VisibilityReport(visible bool) string {
	if visible {
		return "\x1b[?999;1n"
	}
	return "\x1b[?999;2n"
}
