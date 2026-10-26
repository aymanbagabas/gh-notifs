package program

import (
	"image/color"
	"io"

	"altscreen.io/vision/ansi"
	vcolor "altscreen.io/vision/color"
	"altscreen.io/vision/event"
)

var mouseModes = []ansi.Mode{ansi.MouseX10Mode, ansi.MouseNormalMode, ansi.MouseButtonMode, ansi.MouseAnyMode, ansi.MouseSGRMode, ansi.MouseSGRPixelMode}

// SetCursorStyle sets the cursor shape and blinking state.
func (p *Program) SetCursorStyle(shape ansi.CursorShape, blinking bool) error {
	style := ansi.NewCursorStyle(shape, blinking)
	if err := writeString(p.screen, ansi.SetCursorStyle(style)); err != nil {
		return err
	}
	p.state.cursorStyle = style
	return p.screen.Flush()
}

// ResetCursorStyle hands the cursor style back to the terminal with DECSCUSR
// parameter 0.
//
// Every terminal surveyed for this reads its own configuration here: kitty,
// foot, rio, Alacritty, wezterm, VTE, Konsole and Windows Terminal all restore
// the shape their user chose. xterm documents the parameter as a blinking block
// instead, so what appears is the terminal's to decide either way, which is the
// point: the application stops choosing.
//
// That makes this the portable way to undo [Program.SetCursorStyle] without
// knowing what came before. To put back the cursor this session found rather
// than the one the user configured, read it with [Program.RequestCursorStyle]
// before changing it and set that style again.
func (p *Program) ResetCursorStyle() error {
	if err := writeString(p.screen, ansi.SetCursorStyle(ansi.CursorDefault)); err != nil {
		return err
	}
	// Both the reset and the restore paths read CursorDefault as "this session
	// set nothing", so recording it here is what keeps a cursor already handed
	// back from being handed back a second time on teardown.
	p.state.cursorStyle = ansi.CursorDefault
	return p.screen.Flush()
}

// RequestCursorStyle asks the terminal for its current DECSCUSR setting with
// DECRQSS. The reply arrives as a [altscreen.io/vision/event.SettingReportEvent]
// whose Setting is a [altscreen.io/vision/event.CursorStyleSetting].
//
// A terminal that does not implement DECRQSS may report the setting as
// unrecognized or stay silent, so treat the reply as optional. One that
// implements it but names a style this package does not know leaves Setting
// nil, with the reply in Raw.
func (p *Program) RequestCursorStyle() error { return p.writeFlush(ansi.DECRQSS(" q")) }

// Beep rings the terminal bell.
func (p *Program) Beep() error {
	if err := writeString(p.screen, "\a"); err != nil {
		return err
	}
	return p.screen.Flush()
}

// SetPointerShape sets the pointer shape.
func (p *Program) SetPointerShape(shape string) error {
	if err := writeString(p.screen, ansi.SetPointerShape(shape)); err != nil {
		return err
	}
	p.state.pointerShape, p.state.hasPointerShape = shape, true
	return p.screen.Flush()
}

// ResetPointerShape resets the pointer shape.
func (p *Program) ResetPointerShape() error {
	if err := writeString(p.screen, ansi.SetPointerShape("default")); err != nil {
		return err
	}
	p.state.pointerShape, p.state.hasPointerShape = "", false
	return p.screen.Flush()
}

// SetProgressState reports progress to the terminal.
func (p *Program) SetProgressState(progress Progress) error {
	if err := progress.write(p.screen); err != nil {
		return err
	}
	p.state.progress = &progress
	return p.screen.Flush()
}

// ResetProgressState removes the progress report.
func (p *Program) ResetProgressState() error {
	if err := writeString(p.screen, ansi.ResetProgressBar); err != nil {
		return err
	}
	p.state.progress = nil
	return p.screen.Flush()
}

func (progress Progress) write(w io.Writer) error {
	pct := min(max(progress.Percent, 0), 100)
	s := ansi.SetProgressBar(pct)
	switch progress.Kind {
	case ProgressError:
		s = ansi.SetErrorProgressBar(pct)
	case ProgressWarning:
		s = ansi.SetWarningProgressBar(pct)
	case ProgressIndeterminate:
		s = ansi.SetIndeterminateProgressBar
	}
	return writeString(w, s)
}

// EnableMouse enables mouse tracking.
func (p *Program) EnableMouse(tracking MouseTracking) error {
	if err := writeString(p.screen, ansi.ResetMode(mouseModes...)); err != nil {
		return err
	}
	if err := p.writeMouseModes(tracking, true); err != nil {
		return err
	}
	v := tracking
	p.state.mouse = &v
	return p.screen.Flush()
}

// DisableMouse disables mouse tracking.
func (p *Program) DisableMouse() error {
	if err := writeString(p.screen, ansi.ResetMode(mouseModes...)); err != nil {
		return err
	}
	p.state.mouse = nil
	return p.screen.Flush()
}

func (p *Program) writeMouseModes(tracking MouseTracking, enable bool) error {
	modes := []ansi.Mode{ansi.MouseNormalMode, ansi.MouseButtonMode, ansi.MouseSGRMode}
	if tracking.Contains(MouseMotion) {
		modes = append(modes[:2], append([]ansi.Mode{ansi.MouseAnyMode}, modes[2:]...)...)
	}
	if tracking.Contains(MousePixels) {
		modes = append(modes, ansi.MouseSGRPixelMode)
	}
	if enable {
		return writeString(p.screen, ansi.SetMode(modes...))
	}
	return writeString(p.screen, ansi.ResetMode(modes...))
}

// EnableBracketedPaste enables bracketed paste.
func (p *Program) EnableBracketedPaste() error {
	return p.setBoolMode(ansi.BracketedPasteMode, &p.state.bracketedPaste, true)
}

// DisableBracketedPaste disables bracketed paste.
func (p *Program) DisableBracketedPaste() error {
	return p.setBoolMode(ansi.BracketedPasteMode, &p.state.bracketedPaste, false)
}

// EnableFocusEvents enables focus events.
func (p *Program) EnableFocusEvents() error {
	return p.setBoolMode(ansi.FocusMode, &p.state.focusEvents, true)
}

// DisableFocusEvents disables focus events.
func (p *Program) DisableFocusEvents() error {
	return p.setBoolMode(ansi.FocusMode, &p.state.focusEvents, false)
}

// EnableColorSchemeUpdates enables color scheme reports.
func (p *Program) EnableColorSchemeUpdates() error {
	return p.setBoolMode(ansi.LightDarkMode, &p.state.colorScheme, true)
}

// DisableColorSchemeUpdates disables color scheme reports.
func (p *Program) DisableColorSchemeUpdates() error {
	return p.setBoolMode(ansi.LightDarkMode, &p.state.colorScheme, false)
}

// EnableVisibilityReports enables visibility reports.
func (p *Program) EnableVisibilityReports() error {
	return p.setBoolMode(ansi.VisibilityReportsMode, &p.state.visibilityReports, true)
}

// DisableVisibilityReports disables visibility reports.
func (p *Program) DisableVisibilityReports() error {
	return p.setBoolMode(ansi.VisibilityReportsMode, &p.state.visibilityReports, false)
}

// RequestVisibility requests one visibility report.
func (p *Program) RequestVisibility() error {
	if err := writeString(p.screen, ansi.RequestVisibilityReport); err != nil {
		return err
	}
	return p.screen.Flush()
}

// EnableInBandResize enables in-band resize reports.
func (p *Program) EnableInBandResize() error {
	if err := writeString(p.screen, ansi.InBandResizeMode.Set()); err != nil {
		return err
	}
	p.state.inBandResize = true
	p.state.chosen[ansi.InBandResizeMode] = true
	p.source.SetHandleResize(false)
	return p.screen.Flush()
}

// DisableInBandResize disables in-band resize reports.
func (p *Program) DisableInBandResize() error {
	if err := writeString(p.screen, ansi.InBandResizeMode.Reset()); err != nil {
		return err
	}
	p.state.inBandResize = false
	p.state.chosen[ansi.InBandResizeMode] = true
	p.source.SetHandleResize(true)
	return p.screen.Flush()
}

func (p *Program) setBoolMode(mode ansi.Mode, dst *bool, on bool) error {
	seq := mode.Reset()
	if on {
		seq = mode.Set()
	}
	if err := writeString(p.screen, seq); err != nil {
		return err
	}
	*dst = on
	return p.screen.Flush()
}

// SetTitle sets both the window title and icon name.
func (p *Program) SetTitle(title string) error {
	if err := writeString(p.screen, ansi.WindowTitleAndIcon(title)); err != nil {
		return err
	}
	p.state.windowTitle, p.state.iconName = title, title
	p.state.hasWindowTitle, p.state.hasIconName = title != "", title != ""
	return p.screen.Flush()
}

// SetWindowTitle sets the window title.
func (p *Program) SetWindowTitle(title string) error {
	if err := writeString(p.screen, ansi.WindowTitle(title)); err != nil {
		return err
	}
	p.state.windowTitle, p.state.hasWindowTitle = title, title != ""
	return p.screen.Flush()
}

// SetIconTitle sets the icon name.
func (p *Program) SetIconTitle(title string) error {
	if err := writeString(p.screen, ansi.IconName(title)); err != nil {
		return err
	}
	p.state.iconName, p.state.hasIconName = title, title != ""
	return p.screen.Flush()
}

// EnterAltScreen enters the alternate screen buffer.
func (p *Program) EnterAltScreen() error { return p.setAltScreen(true) }

// ExitAltScreen leaves the alternate screen buffer.
func (p *Program) ExitAltScreen() error { return p.setAltScreen(false) }

func (p *Program) setAltScreen(enter bool) error {
	changed := p.state.altScreen != enter
	if changed && enter {
		p.screen.SaveCursor()
	}
	seq := ansi.AltScreenSaveCursorMode.Reset()
	if enter {
		seq = ansi.AltScreenSaveCursorMode.Set()
	}
	if err := writeString(p.screen, seq); err != nil {
		return err
	}
	p.state.altScreen = enter
	if changed {
		p.screen.SetFullscreen(enter)
		if p.state.cursorVisible {
			_ = writeString(p.screen, ansi.CursorVisibleMode.Set())
		} else {
			_ = writeString(p.screen, ansi.CursorVisibleMode.Reset())
		}
		if p.state.kittyKeyboard != 0 {
			_ = writeString(p.screen, ansi.SetKittyKeyboard(p.state.kittyKeyboard, ansi.KittyKeyboardSet))
		}
	}
	return p.screen.Flush()
}

// ShowCursor shows the terminal cursor.
func (p *Program) ShowCursor() error {
	p.screen.SetCursorVisible(true)
	return p.setCursorVisible(true)
}

// HideCursor hides the terminal cursor.
func (p *Program) HideCursor() error {
	p.screen.SetCursorVisible(false)
	return p.setCursorVisible(false)
}

func (p *Program) setCursorVisible(v bool) error {
	seq := ansi.CursorVisibleMode.Reset()
	if v {
		seq = ansi.CursorVisibleMode.Set()
	}
	if err := writeString(p.screen, seq); err != nil {
		return err
	}
	p.state.cursorVisible = v
	return p.screen.Flush()
}

// EnableGraphemeClusters enables Unicode core mode.
func (p *Program) EnableGraphemeClusters() error {
	if err := writeString(p.screen, ansi.UnicodeCoreMode.Set()); err != nil {
		return err
	}
	p.state.graphemeClusters = true
	p.state.chosen[ansi.UnicodeCoreMode] = true
	p.screen.SetGraphemeClusters(true)
	return p.screen.Flush()
}

// DisableGraphemeClusters disables Unicode core mode.
func (p *Program) DisableGraphemeClusters() error {
	if err := writeString(p.screen, ansi.UnicodeCoreMode.Reset()); err != nil {
		return err
	}
	p.state.graphemeClusters = false
	p.state.chosen[ansi.UnicodeCoreMode] = true
	p.screen.SetGraphemeClusters(false)
	return p.screen.Flush()
}

// SetKittyKeyboard sets Kitty keyboard flags.
func (p *Program) SetKittyKeyboard(flags *ansi.KittyKeyboardFlags) error {
	f := ansi.KittyKeyboardFlags(0)
	if flags != nil {
		f = *flags
	}
	if err := writeString(p.screen, ansi.SetKittyKeyboard(f, ansi.KittyKeyboardSet)); err != nil {
		return err
	}
	p.state.kittyKeyboard = f
	return p.screen.Flush()
}

// BackspaceMode returns the decoder's current Backarrow mode (DECBKM).
// ObserveEvent applies recognized reports to future decoding.
// Events already in the queue retain their decoded values.
func (p *Program) BackspaceMode() bool { return p.source.BackspaceMode() }

// SetBackspaceMode writes and flushes Backarrow mode, then updates legacy decoding.
// Set (true) selects BS for Backspace; reset (false) selects DEL.
// DecoderFlags control ambiguous interpretations. A terminal can ignore the request.
//
// Pause resets an application-selected mode. Resume restores the selection.
// Reports update decoding without changing that selection or the caller's options.
func (p *Program) SetBackspaceMode(enabled bool) error {
	if err := p.writeBackspaceMode(enabled); err != nil {
		return err
	}
	p.state.backspaceMode = enabled
	p.state.chosen[ansi.BackarrowKeyMode] = true
	return nil
}

func (p *Program) writeBackspaceMode(enabled bool) error {
	seq := ansi.BackarrowKeyMode.Reset()
	if enabled {
		seq = ansi.BackarrowKeyMode.Set()
	}
	if err := p.writeFlush(seq); err != nil {
		return err
	}
	p.source.SetBackspaceMode(enabled)
	return nil
}

// SetModifyOtherKeys sets xterm modifyOtherKeys.
func (p *Program) SetModifyOtherKeys(mode event.ModifyOtherKeysMode) error {
	seq := ansi.ResetModifyOtherKeys
	if mode == event.ModifyOtherKeysMode1 {
		v := 1
		seq = ansi.SetKeyModifierOptions(4, &v)
	}
	if mode == event.ModifyOtherKeysMode2 {
		v := 2
		seq = ansi.SetKeyModifierOptions(4, &v)
	}
	if err := writeString(p.screen, seq); err != nil {
		return err
	}
	p.state.modifyOtherKeys = mode
	return p.screen.Flush()
}

// SetForegroundColor sets the default foreground color.
func (p *Program) SetForegroundColor(c color.Color) error {
	p.state.foregroundColor = c
	return p.setColor(ansi.SetForegroundColor, c)
}

// ResetForegroundColor resets the default foreground color.
func (p *Program) ResetForegroundColor() error {
	p.state.foregroundColor = nil
	return p.writeFlush(ansi.ResetForegroundColor)
}

// SetBackgroundColor sets the default background color.
func (p *Program) SetBackgroundColor(c color.Color) error {
	p.state.backgroundColor = c
	return p.setColor(ansi.SetBackgroundColor, c)
}

// ResetBackgroundColor resets the default background color.
func (p *Program) ResetBackgroundColor() error {
	p.state.backgroundColor = nil
	return p.writeFlush(ansi.ResetBackgroundColor)
}

// SetCursorColor sets the cursor color.
func (p *Program) SetCursorColor(c color.Color) error {
	p.state.cursorColor = c
	return p.setColor(ansi.SetCursorColor, c)
}

// ResetCursorColor resets the cursor color.
func (p *Program) ResetCursorColor() error {
	p.state.cursorColor = nil
	return p.writeFlush(ansi.ResetCursorColor)
}

func (p *Program) setColor(fn func(string) string, c color.Color) error {
	rgb := vcolor.ToRGB(c)
	return p.writeFlush(fn(ansi.XParseRGB(rgb.R, rgb.G, rgb.B)))
}

// SetPaletteColor sets a terminal palette color.
func (p *Program) SetPaletteColor(index int, c color.Color) error {
	p.state.palette[index] = c
	rgb := vcolor.ToRGB(c)
	return p.writeFlush(ansi.SetPaletteColor(uint8(index), ansi.XParseRGB(rgb.R, rgb.G, rgb.B)))
}

// ResetPaletteColor resets one terminal palette color.
func (p *Program) ResetPaletteColor(index int) error {
	delete(p.state.palette, index)
	return p.writeFlush(ansi.ResetPaletteColor(uint8(index)))
}

// ResetPaletteColors resets all terminal palette colors.
func (p *Program) ResetPaletteColors() error {
	p.state.palette = map[int]color.Color{}
	return p.writeFlush(ansi.ResetPaletteColors)
}

// RequestWindowPixelSize requests the window size in pixels.
func (p *Program) RequestWindowPixelSize() error { return p.writeFlush(ansi.RequestWindowPixelSize) }

// RequestCellPixelSize requests the character cell size in pixels.
func (p *Program) RequestCellPixelSize() error { return p.writeFlush(ansi.RequestCellPixelSize) }

// RequestOrigin requests the managed area's physical origin.
func (p *Program) RequestOrigin() error {
	if p.screen.Fullscreen() {
		return nil
	}
	_ = p.screen.MoveCursorTo(0, 0)
	if err := writeString(p.screen, ansi.RequestCursorPosition); err != nil {
		return err
	}
	p.originQueriesPending++
	return p.screen.Flush()
}

// RequestKittyKeyboard requests active Kitty keyboard flags.
func (p *Program) RequestKittyKeyboard() error { return p.writeFlush(ansi.RequestKittyKeyboard) }

// RequestModifyOtherKeys requests modifyOtherKeys state.
func (p *Program) RequestModifyOtherKeys() error { return p.writeFlush(ansi.QueryModifyOtherKeys) }

// RequestForegroundColor requests the default foreground color.
func (p *Program) RequestForegroundColor() error { return p.writeFlush(ansi.RequestForegroundColor) }

// RequestBackgroundColor requests the default background color.
func (p *Program) RequestBackgroundColor() error { return p.writeFlush(ansi.RequestBackgroundColor) }

// RequestCursorColor requests the cursor color.
func (p *Program) RequestCursorColor() error { return p.writeFlush(ansi.RequestCursorColor) }

// RequestPaletteColor requests a palette color.
func (p *Program) RequestPaletteColor(index int) error {
	return p.writeFlush(ansi.RequestPaletteColor(uint8(index)))
}

// RequestMode requests a mode report.
func (p *Program) RequestMode(mode ansi.Mode) error { return p.writeFlush(mode.Request()) }

// RequestTerminalVersion requests the terminal's name and version, which
// arrives as an [event.TerminalNameEvent].
func (p *Program) RequestTerminalVersion() error { return p.writeFlush(ansi.RequestXTVersion) }

// RequestTermcap requests terminfo capability entries by name, which arrive as
// an [event.TermcapEvent]. Asking for nothing sends nothing.
func (p *Program) RequestTermcap(names ...string) error {
	if len(names) == 0 {
		return nil
	}
	return p.writeFlush(ansi.XTGETTCAP(names...))
}

// RequestCursorPosition requests the cursor position.
func (p *Program) RequestCursorPosition() error { return p.writeFlush(ansi.RequestCursorPosition) }

// RequestExtendedCursorPosition requests the cursor position without F3 ambiguity.
// The reply is a [event.CursorPositionEvent] with a page number when reported.
func (p *Program) RequestExtendedCursorPosition() error {
	return p.writeFlush(ansi.RequestExtendedCursorPosition)
}

// RequestColorScheme requests the current color scheme.
func (p *Program) RequestColorScheme() error { return p.writeFlush(ansi.RequestLightDarkReport) }

// SetSystemClipboard sets the system clipboard.
func (p *Program) SetSystemClipboard(data []byte) error {
	return p.writeFlush(ansi.SetClipboard(ansi.SystemClipboard, data))
}

// SetPrimaryClipboard sets the primary selection.
func (p *Program) SetPrimaryClipboard(data []byte) error {
	return p.writeFlush(ansi.SetClipboard(ansi.PrimaryClipboard, data))
}

// RequestSystemClipboard requests the system clipboard.
func (p *Program) RequestSystemClipboard() error {
	return p.writeFlush(ansi.RequestClipboard(ansi.SystemClipboard))
}

// RequestPrimaryClipboard requests the primary selection.
func (p *Program) RequestPrimaryClipboard() error {
	return p.writeFlush(ansi.RequestClipboard(ansi.PrimaryClipboard))
}

// QueryCapabilities writes the standard capability queries, selected optional
// queries, extra, and a final Primary DA request, then flushes once.
// Options controls optional queries. Palette requests retain their supplied order.
func (p *Program) QueryCapabilities(extra []byte) error {
	if err := writeString(p.screen, ansi.RequestKittyKeyboard); err != nil {
		return err
	}
	if !p.isAppleTerminal() {
		for _, mode := range []ansi.Mode{ansi.SynchronizedOutputMode, ansi.UnicodeCoreMode, ansi.InBandResizeMode, ansi.VisibilityReportsMode, ansi.MouseNormalMode, ansi.MouseButtonMode, ansi.MouseAnyMode, ansi.MouseSGRMode, ansi.MouseSGRPixelMode} {
			_ = writeString(p.screen, mode.Request())
		}
		if p.options.QueryBackspaceMode {
			_ = writeString(p.screen, ansi.BackarrowKeyMode.Request())
		}
		_ = writeString(p.screen, ansi.RequestXTVersion)
		_ = writeString(p.screen, ansi.QueryModifyOtherKeys)
		if p.screen.ColorProfile() < vcolor.TrueColor {
			_ = writeString(p.screen, ansi.XTGETTCAP("RGB"))
			_ = writeString(p.screen, ansi.XTGETTCAP("Tc"))
		}
	} else if p.screen.ColorProfile() < vcolor.TrueColor {
		if v, ok := p.env.LookupEnv("TERM_PROGRAM_VERSION"); ok && appleVersion(v) >= 470 {
			p.screen.SetColorProfile(vcolor.TrueColor)
		}
	}
	if p.options.QueryForegroundColor {
		if _, err := ansi.WriteRequestForegroundColor(p.screen); err != nil {
			return err
		}
	}
	if p.options.QueryBackgroundColor {
		if _, err := ansi.WriteRequestBackgroundColor(p.screen); err != nil {
			return err
		}
	}
	if p.options.QueryCursorColor {
		if _, err := ansi.WriteRequestCursorColor(p.screen); err != nil {
			return err
		}
	}
	for _, index := range p.options.QueryPaletteColors {
		if _, err := ansi.WriteRequestPaletteColor(p.screen, index); err != nil {
			return err
		}
	}
	if p.options.QueryProgramStatus {
		if _, err := ansi.WriteRequestProgramStatus(p.screen); err != nil {
			return err
		}
	}
	_, _ = p.screen.Write(extra)
	if err := writeString(p.screen, ansi.RequestPrimaryDA); err != nil {
		return err
	}
	return p.screen.Flush()
}

// resetTabStops resets hardware tab stops to every eight columns.
//
// The stops belong to the terminal rather than to this program's willingness
// to use them, so this runs whether or not the tab optimizations are on:
// turning them on later must not find the stops unknown.
func (p *Program) resetTabStops() error {
	return p.writeFlush(ansi.ResetTabStopsEvery8(p.screen.Size().Width))
}

// resetLNM resets line-feed/new-line mode, which a suspended program may have
// had set behind its back.
func (p *Program) resetLNM() error { return p.writeFlush(ansi.LineFeedNewLineMode.Reset()) }

func (p *Program) reset() error {
	if p.state.cursorStyle != ansi.CursorDefault {
		_ = writeString(p.screen, ansi.SetCursorStyle(ansi.CursorDefault))
	}
	if p.state.bracketedPaste {
		_ = writeString(p.screen, ansi.BracketedPasteMode.Reset())
	}
	if p.state.focusEvents {
		_ = writeString(p.screen, ansi.FocusMode.Reset())
	}
	if p.state.mouse != nil {
		_ = p.writeMouseModes(*p.state.mouse, false)
	}
	if p.state.colorScheme {
		_ = writeString(p.screen, ansi.LightDarkMode.Reset())
	}
	if p.state.visibilityReports {
		_ = writeString(p.screen, ansi.VisibilityReportsMode.Reset())
	}
	if p.state.inBandResize {
		_ = writeString(p.screen, ansi.InBandResizeMode.Reset())
	}
	if p.state.chosen[ansi.BackarrowKeyMode] {
		if err := p.writeBackspaceMode(false); err != nil {
			return err
		}
	}
	if p.state.modifyOtherKeys != event.ModifyOtherKeysDisabled {
		_ = writeString(p.screen, ansi.ResetModifyOtherKeys)
	}
	if p.state.foregroundColor != nil {
		_ = writeString(p.screen, ansi.ResetForegroundColor)
	}
	if p.state.backgroundColor != nil {
		_ = writeString(p.screen, ansi.ResetBackgroundColor)
	}
	if p.state.cursorColor != nil {
		_ = writeString(p.screen, ansi.ResetCursorColor)
	}
	if len(p.state.palette) > 0 {
		_ = writeString(p.screen, ansi.ResetPaletteColors)
	}
	if p.state.hasWindowTitle && p.state.hasIconName && p.state.windowTitle == p.state.iconName {
		_ = writeString(p.screen, ansi.WindowTitleAndIcon(""))
	} else {
		if p.state.hasWindowTitle {
			_ = writeString(p.screen, ansi.WindowTitle(""))
		}
		if p.state.hasIconName {
			_ = writeString(p.screen, ansi.IconName(""))
		}
	}
	if p.state.hasPointerShape {
		_ = writeString(p.screen, ansi.SetPointerShape("default"))
	}
	if p.state.progress != nil {
		_ = writeString(p.screen, ansi.ResetProgressBar)
	}
	if err := p.screen.ParkCursor(); err != nil {
		return err
	}
	if !p.state.cursorVisible {
		_ = writeString(p.screen, ansi.CursorVisibleMode.Set())
	}
	fullscreen := p.state.altScreen
	if fullscreen && p.state.kittyKeyboard != 0 {
		_ = writeString(p.screen, ansi.SetKittyKeyboard(0, ansi.KittyKeyboardSet))
	}
	if fullscreen {
		_ = writeString(p.screen, ansi.AltScreenSaveCursorMode.Reset())
		p.screen.RestoreCursor()
	}
	if p.state.kittyKeyboard != 0 {
		_ = writeString(p.screen, ansi.SetKittyKeyboard(0, ansi.KittyKeyboardSet))
	}
	if p.state.graphemeClusters {
		_ = writeString(p.screen, ansi.UnicodeCoreMode.Reset())
	}
	p.screen.InvalidateTrackedCursor()
	return nil
}

func (p *Program) restore() error {
	if p.state.kittyKeyboard != 0 {
		_ = writeString(p.screen, ansi.SetKittyKeyboard(p.state.kittyKeyboard, ansi.KittyKeyboardSet))
	}
	fullscreen := p.state.altScreen
	if fullscreen {
		p.screen.SaveCursor()
		_ = writeString(p.screen, ansi.AltScreenSaveCursorMode.Set())
	}
	if fullscreen && p.state.kittyKeyboard != 0 {
		_ = writeString(p.screen, ansi.SetKittyKeyboard(p.state.kittyKeyboard, ansi.KittyKeyboardSet))
	}
	if p.state.graphemeClusters {
		_ = writeString(p.screen, ansi.UnicodeCoreMode.Set())
	}
	if !p.state.cursorVisible {
		_ = writeString(p.screen, ansi.CursorVisibleMode.Reset())
	}
	if p.state.cursorStyle != ansi.CursorDefault {
		_ = writeString(p.screen, ansi.SetCursorStyle(p.state.cursorStyle))
	}
	if p.state.colorScheme {
		_ = writeString(p.screen, ansi.LightDarkMode.Set())
	}
	if p.state.visibilityReports {
		_ = writeString(p.screen, ansi.VisibilityReportsMode.Set())
	}
	if p.state.inBandResize {
		_ = writeString(p.screen, ansi.InBandResizeMode.Set())
	}
	if p.state.chosen[ansi.BackarrowKeyMode] {
		if err := p.writeBackspaceMode(p.state.backspaceMode); err != nil {
			return err
		}
	}
	if p.state.modifyOtherKeys != event.ModifyOtherKeysDisabled {
		_ = p.SetModifyOtherKeys(p.state.modifyOtherKeys)
	}
	if p.state.bracketedPaste {
		_ = writeString(p.screen, ansi.BracketedPasteMode.Set())
	}
	if p.state.focusEvents {
		_ = writeString(p.screen, ansi.FocusMode.Set())
	}
	if p.state.mouse != nil {
		_ = p.writeMouseModes(*p.state.mouse, true)
	}
	if p.state.foregroundColor != nil {
		_ = p.setColorNoFlush(ansi.SetForegroundColor, p.state.foregroundColor)
	}
	if p.state.backgroundColor != nil {
		_ = p.setColorNoFlush(ansi.SetBackgroundColor, p.state.backgroundColor)
	}
	if p.state.cursorColor != nil {
		_ = p.setColorNoFlush(ansi.SetCursorColor, p.state.cursorColor)
	}
	for i, c := range p.state.palette {
		rgb := vcolor.ToRGB(c)
		_ = writeString(p.screen, ansi.SetPaletteColor(uint8(i), ansi.XParseRGB(rgb.R, rgb.G, rgb.B)))
	}
	if p.state.hasWindowTitle && p.state.hasIconName && p.state.windowTitle == p.state.iconName {
		_ = writeString(p.screen, ansi.WindowTitleAndIcon(p.state.windowTitle))
	} else {
		if p.state.hasWindowTitle {
			_ = writeString(p.screen, ansi.WindowTitle(p.state.windowTitle))
		}
		if p.state.hasIconName {
			_ = writeString(p.screen, ansi.IconName(p.state.iconName))
		}
	}
	if p.state.hasPointerShape {
		_ = writeString(p.screen, ansi.SetPointerShape(p.state.pointerShape))
	}
	if p.state.progress != nil {
		_ = p.state.progress.write(p.screen)
	}
	return nil
}

func (p *Program) writeFlush(s string) error {
	if err := writeString(p.screen, s); err != nil {
		return err
	}
	return p.screen.Flush()
}

func (p *Program) setColorNoFlush(fn func(string) string, c color.Color) error {
	rgb := vcolor.ToRGB(c)
	return writeString(p.screen, fn(ansi.XParseRGB(rgb.R, rgb.G, rgb.B)))
}

func (p *Program) isAppleTerminal() bool {
	v, _ := p.env.LookupEnv("TERM_PROGRAM")
	return v == "Apple_Terminal"
}

func appleVersion(v string) int {
	n := 0
	for _, r := range v {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
