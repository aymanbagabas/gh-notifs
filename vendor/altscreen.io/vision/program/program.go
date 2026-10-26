// Package program provides the managed terminal session facade.
package program

import (
	"context"
	"io"
	"slices"
	"time"

	"altscreen.io/vision/ansi"
	vcolor "altscreen.io/vision/color"
	"altscreen.io/vision/event"
	"altscreen.io/vision/layout"
	"altscreen.io/vision/screen"
	"altscreen.io/vision/term"
)

// MouseTracking selects optional mouse tracking features.
type MouseTracking uint8

const (
	// MouseMotion reports pointer motion without a button held.
	MouseMotion MouseTracking = 1 << iota
	// MousePixels requests SGR-pixel coordinates.
	MousePixels
)

// Contains reports whether all flags are enabled.
func (m MouseTracking) Contains(flags MouseTracking) bool { return m&flags == flags }

// ProgressKind selects an OSC 9;4 progress state.
type ProgressKind int

const (
	// ProgressNormal is determinate progress.
	ProgressNormal ProgressKind = iota
	// ProgressError is failed progress.
	ProgressError
	// ProgressWarning is attention-needed progress.
	ProgressWarning
	// ProgressIndeterminate is progress of unknown duration.
	ProgressIndeterminate
)

// Progress is a terminal progress report.
type Progress struct {
	// Kind selects the emitted progress state.
	Kind ProgressKind
	// Percent is clamped to 0..100 for determinate states.
	Percent int
}

// Options configures session setup, optional queries, and mode preferences.
// InitWith applies BracketedPaste, Mouse, and LegacyKeys.
// QueryCapabilities sends the optional queries. ObserveEvent applies mode
// preferences when a reply confirms support.
type Options struct {
	// BracketedPaste enables bracketed paste at init.
	BracketedPaste bool
	// Mouse enables mouse tracking at init when non-nil.
	Mouse *MouseTracking
	// PreferGraphemeClusters adopts Unicode core mode when reported.
	PreferGraphemeClusters bool
	// PreferInBandResize adopts in-band resize mode when reported.
	PreferInBandResize bool
	// PreferSynchronizedOutput adopts synchronized output when reported.
	PreferSynchronizedOutput bool
	// QueryBackspaceMode includes Backarrow mode in QueryCapabilities.
	// It defaults to false. InitWith sends no queries.
	// ObserveEvent applies recognized reports regardless of this option.
	QueryBackspaceMode bool
	// QueryForegroundColor includes the default foreground in QueryCapabilities.
	// It defaults to false.
	QueryForegroundColor bool
	// QueryBackgroundColor includes the default background in QueryCapabilities.
	// It defaults to false.
	QueryBackgroundColor bool
	// QueryCursorColor includes the cursor color in QueryCapabilities.
	// It defaults to false.
	QueryCursorColor bool
	// QueryPaletteColors selects palette entries for QueryCapabilities.
	// The default is empty. Requests preserve the supplied order and duplicates.
	QueryPaletteColors []uint8
	// QueryProgramStatus includes OSC 7501 support in QueryCapabilities.
	// It defaults to false. Initialization sends no queries.
	// Reports and record lifetime do not depend on this option.
	QueryProgramStatus bool
	// LegacyKeys chooses how the ambiguous legacy keys are read.
	//
	// LF defaults to Ctrl+J; [event.DecoderFlagLFIsEnter] selects Enter instead.
	// See [event.DecoderFlags] for all mappings.
	// Use [Program.SetDecoderFlags] to change the active flags after initialization.
	LegacyKeys event.DecoderFlags
}

// DefaultOptions returns the default session, query, and mode options.
func DefaultOptions() Options {
	return Options{BracketedPaste: true, PreferGraphemeClusters: true, PreferInBandResize: true, PreferSynchronizedOutput: true}
}

// Program owns a terminal, an event source, and a screen renderer.
type Program struct {
	terminal             *term.Terminal
	env                  term.Environ
	screen               *screen.Screen
	source               *event.Source
	unread               []event.Event
	state                state
	caps                 Capabilities
	options              Options
	windowCells          *layout.Size
	windowPixels         *layout.Size
	cellPixels           *layout.Size
	origin               layout.Position
	originQueriesPending int
}

// New constructs a Program over terminal without entering raw mode.
//
// The terminal has to report its size, because that size is what the managed
// area is built from. A caller that wants to carry on without one can measure
// it however it likes and fall back to [term.DefaultWinsize].
func New(terminal *term.Terminal) (*Program, error) {
	ws, err := terminal.GetWindowSize()
	if err != nil {
		return nil, err
	}
	return withRender(terminal, layout.Size{Width: ws.Col, Height: ws.Row}), nil
}

// Stdio constructs a Program over process stdin and stdout.
func Stdio() (*Program, error) {
	t, err := term.Stdio()
	if err != nil {
		return nil, err
	}
	return New(t)
}

// Open constructs a Program over the controlling terminal.
func Open() (*Program, error) {
	t, err := term.Open()
	if err != nil {
		return nil, err
	}
	return New(t)
}

func withRender(t *term.Terminal, size layout.Size) *Program {
	p := withParts(t.Input(), t.Output(), t.Env(), size)
	p.terminal = t
	return p
}

// withParts assembles a Program from the pieces it actually renders with,
// without acquiring a terminal. Terminal acquisition is term.New's job and it
// requires a real terminal; this path exists so the rendering and mode logic
// can be exercised over ordinary readers and writers.
//
// A Program built this way has no Terminal, so the session lifecycle is not
// available on it.
func withParts(in io.Reader, out io.Writer, env term.Environ, size layout.Size) *Program {
	s := screen.New(out, size)
	s.SetColorProfile(vcolor.DetectFrom(env, true))
	s.SetOptimizations(screen.OptimizationsFromEnv(env))
	source, _ := event.NewSource(in)
	return &Program{env: env, screen: s, source: source, state: defaultState(), caps: NewCapabilities(), options: DefaultOptions()}
}

// Terminal returns the terminal this program drives.
func (p *Program) Terminal() *term.Terminal { return p.terminal }

// Env returns the environment this program reads terminal settings from.
func (p *Program) Env() term.Environ { return p.env }

// Screen returns the renderer this program owns.
func (p *Program) Screen() *screen.Screen { return p.screen }

// Init begins a session with default options.
func (p *Program) Init() error { return p.InitWith(DefaultOptions()) }

// InitWith begins a session and stores options for later queries and mode reports.
// It sends no capability queries. Call QueryCapabilities to send them.
func (p *Program) InitWith(options Options) error {
	p.options = options
	// The decoder is the only thing that can act on this, and it is behind the
	// source, so the option is carried there as it is taken.
	p.SetDecoderFlags(options.LegacyKeys)
	if err := p.makeRaw(); err != nil {
		return err
	}
	p.enableTabsAndBS()
	if err := p.resetLNM(); err != nil {
		return err
	}
	if err := p.Autoresize(); err != nil {
		return err
	}
	outTTY := false
	if p.terminal != nil {
		_, outTTY = p.terminal.IsTerminal()
	}
	p.screen.SetColorProfile(vcolor.DetectFrom(p.env, outTTY))
	if err := p.resetTabStops(); err != nil {
		return err
	}
	if options.BracketedPaste {
		if err := p.EnableBracketedPaste(); err != nil {
			return err
		}
	}
	if options.Mouse != nil {
		if err := p.EnableMouse(*options.Mouse); err != nil {
			return err
		}
	}
	return nil
}

// Finish tears down the session and restores the terminal state.
func (p *Program) Finish() error {
	teardown := p.teardown()
	restore := p.restoreTerminal()
	// The source may hold a readiness backend and the descriptors it watches.
	p.source.Close()
	if teardown != nil {
		return teardown
	}
	return restore
}

// Pause tears down modes and restores terminal state without consuming p.
func (p *Program) Pause() error {
	teardown := p.teardown()
	restore := p.restoreTerminal()
	if teardown != nil {
		return teardown
	}
	return restore
}

// Resume re-enters raw mode, restores modes, and invalidates the screen.
func (p *Program) Resume() error {
	if err := p.makeRaw(); err != nil {
		return err
	}
	p.enableTabsAndBS()
	if err := p.resetLNM(); err != nil {
		return err
	}
	if err := p.Autoresize(); err != nil {
		return err
	}
	if err := p.resetTabStops(); err != nil {
		return err
	}
	if err := p.restore(); err != nil {
		return err
	}
	p.screen.Invalidate()
	return p.screen.Flush()
}

// GetWindowSize returns the live terminal size.
func (p *Program) GetWindowSize() (term.Winsize, error) {
	if p.terminal == nil {
		return term.Winsize{}, term.ErrNotTerminal
	}
	return p.terminal.GetWindowSize()
}

// restoreTerminal puts the terminal back, and does nothing for a Program
// assembled from parts, which owns no terminal to restore.
func (p *Program) restoreTerminal() error {
	if p.terminal == nil {
		return nil
	}
	return p.terminal.Restore()
}

// makeRaw enters raw mode, and does nothing for a Program assembled from
// parts.
func (p *Program) makeRaw() error {
	if p.terminal == nil {
		return nil
	}
	_, err := p.terminal.MakeRaw()
	return err
}

// Autoresize refits the managed area to the terminal size.
//
// When the managed area already fits the queried size, this returns without
// resizing and without repainting, so it is safe to call on every resize
// report. Terminals send a report per pixel of a window drag while the cell
// grid only changes at cell boundaries, so most reports ask for a size the area
// already has. To re-establish the area whatever the size, call
// [altscreen.io/vision/screen.Screen.Resize] instead.
func (p *Program) Autoresize() error {
	ws, err := p.GetWindowSize()
	if err != nil {
		// Keep the size already in hand rather than collapsing the managed
		// area to nothing because one query failed.
		return nil
	}
	p.cacheWindowSize(ws)
	h := p.screen.Size().Height
	if p.screen.Fullscreen() {
		h = ws.Row
	}
	// Terminals report resizes for changes that leave the cell grid alone: a
	// font change that keeps rows and columns, a window move on some
	// terminals. This runs on every one of them, and Screen.Resize repaints
	// unconditionally, so skip the ones that change nothing rather than
	// flickering on each. A caller that wants the repaint anyway calls
	// Screen.Resize directly.
	if p.screen.Size() == (layout.Size{Width: ws.Col, Height: h}) {
		return nil
	}
	p.screen.Resize(layout.Size{Width: ws.Col, Height: h})
	return nil
}

// Capabilities returns terminal replies observed so far.
func (p *Program) Capabilities() *Capabilities { return &p.caps }

// WindowCells returns the last observed terminal cell size.
func (p *Program) WindowCells() (layout.Size, bool) {
	if p.windowCells == nil {
		return layout.Size{}, false
	}
	return *p.windowCells, true
}

// WindowPixels returns the last observed terminal pixel size.
func (p *Program) WindowPixels() (layout.Size, bool) {
	if p.windowPixels == nil {
		return layout.Size{}, false
	}
	return *p.windowPixels, true
}

// CellPixels returns the reported or derived cell pixel size.
func (p *Program) CellPixels() (layout.Size, bool) {
	if p.cellPixels != nil && p.cellPixels.Width > 0 && p.cellPixels.Height > 0 {
		return *p.cellPixels, true
	}
	if p.windowPixels == nil || p.windowCells == nil || p.windowCells.Width == 0 || p.windowCells.Height == 0 {
		return layout.Size{}, false
	}
	s := layout.Size{Width: p.windowPixels.Width / p.windowCells.Width, Height: p.windowPixels.Height / p.windowCells.Height}
	return s, s.Width > 0 && s.Height > 0
}

// TerminalName returns the observed terminal name.
func (p *Program) TerminalName() (string, bool) { return p.caps.TerminalName() }

// MousePixelsToCells converts a pixel mouse event to cell coordinates.
func (p *Program) MousePixelsToCells(m event.Mouse) (event.Mouse, bool) {
	if p.cellPixels != nil && p.cellPixels.Width > 0 && p.cellPixels.Height > 0 {
		return event.NewMouse(m.X/p.cellPixels.Width, m.Y/p.cellPixels.Height, m.Button, m.Mod), true
	}
	if p.windowPixels == nil || p.windowCells == nil || p.windowPixels.Width == 0 || p.windowPixels.Height == 0 {
		return event.Mouse{}, false
	}
	return event.MousePixelToCell(m, p.windowPixels.Width, p.windowPixels.Height, p.windowCells.Width, p.windowCells.Height), true
}

// Origin returns the managed area's tracked physical origin.
func (p *Program) Origin() layout.Position {
	if p.screen.Fullscreen() {
		return layout.Pos(0, 0)
	}
	return p.origin
}

// MouseToOrigin maps a mouse event into managed-area coordinates.
func (p *Program) MouseToOrigin(m event.Mouse) event.Mouse {
	o := p.Origin()
	return event.NewMouse(max(m.X-o.X, 0), max(m.Y-o.Y, 0), m.Button, m.Mod)
}

// DecoderFlags returns the source's current legacy key interpretations.
func (p *Program) DecoderFlags() event.DecoderFlags { return p.source.DecoderFlags() }

// SetDecoderFlags selects interpretations for future legacy key decoding.
// It leaves queued events and initialization options unchanged.
// It does not change terminal modes.
func (p *Program) SetDecoderFlags(flags event.DecoderFlags) { p.source.SetDecoderFlags(flags) }

// PollEvent reports whether an event is immediately available.
func (p *Program) PollEvent(timeout time.Duration) (bool, error) {
	if len(p.unread) > 0 {
		return true, nil
	}
	return p.source.Poll(timeout)
}

// TryReadEvent returns a queued event without reading more input.
// It returns nil, nil when no event is queued. If observation fails, it
// consumes the event and returns nil with the error.
//
// ObserveEvent can enable preferred terminal modes and flush output.
// Events returned through UnreadEvent were already observed.
func (p *Program) TryReadEvent() (event.Event, error) {
	if len(p.unread) > 0 {
		ev := p.unread[0]
		p.unread = p.unread[1:]
		return ev, nil
	}
	ev, ok := p.source.TryRead()
	if !ok {
		return nil, nil
	}
	if err := p.ObserveEvent(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// ReadEvent blocks until the next event is read.
// A successful read always returns a non-nil event.
func (p *Program) ReadEvent() (event.Event, error) {
	if ev, err := p.TryReadEvent(); ev != nil || err != nil {
		return ev, err
	}
	ev, err := p.source.Read()
	if err != nil {
		return nil, err
	}
	if err := p.ObserveEvent(ev); err != nil {
		return nil, err
	}
	return ev, nil
}

// UnreadEvent returns an already observed event to the front of the queue.
// It panics if ev is nil.
func (p *Program) UnreadEvent(ev event.Event) {
	if ev == nil {
		panic("program: cannot unread a nil event")
	}
	p.unread = append([]event.Event{ev}, p.unread...)
}

// Source returns the event source the program reads from, for a caller that
// wants to drive it directly: to poll it on its own schedule, or to reach an
// escape hatch such as [event.Source.EndPaste].
//
// Events taken this way bypass the program, so capability tracking does not run
// on them. Pass each one to [Program.ObserveEvent] to keep that up to date.
//
// Sharing one source between a direct reader and the program's own
// [Program.ReadEvent] is best effort: an event goes to whichever consumer
// drains it first, so settle on one reader.
func (p *Program) Source() *event.Source { return p.source }

// Events streams events until ctx is done or input reaches EOF.
// Do not read from Events and the poller methods concurrently on one Program.
func (p *Program) Events(ctx context.Context) <-chan event.Event {
	ch := make(chan event.Event)
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			ev, err := p.ReadEvent()
			if err != nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case ch <- ev:
			}
		}
	}()
	return ch
}

// ObserveEvent records capability and geometry information from ev.
func (p *Program) ObserveEvent(ev event.Event) error {
	p.caps.init()
	switch e := ev.(type) {
	case event.BackgroundColorEvent:
		p.caps.backgroundColor = e.Color
	case event.ForegroundColorEvent:
		p.caps.foregroundColor = e.Color
	case event.CursorColorEvent:
		p.caps.cursorColor = e.Color
	case event.ModeReportEvent:
		p.caps.modes[e.Mode] = e.Setting
		if e.Mode == ansi.BackarrowKeyMode && e.Setting.IsRecognized() {
			p.source.SetBackspaceMode(e.Setting.IsSet())
		}
		if e.Setting.IsAvailable() && !p.state.chosen[e.Mode] {
			switch e.Mode {
			case ansi.SynchronizedOutputMode:
				if p.options.PreferSynchronizedOutput {
					p.screen.SetSynchronizedOutput(true)
				}
				p.state.chosen[e.Mode] = true
			case ansi.UnicodeCoreMode:
				if p.options.PreferGraphemeClusters {
					return p.EnableGraphemeClusters()
				}
			case ansi.InBandResizeMode:
				if p.options.PreferInBandResize {
					return p.EnableInBandResize()
				}
			}
		}
	case event.KittyKeyboardEnhancementsEvent:
		v := ansi.KittyKeyboardFlags(e)
		p.caps.kittyKeyboard = &v
	case event.ModifyOtherKeysEvent:
		v := event.ModifyOtherKeysMode(e)
		p.caps.modifyOtherKeys = &v
	case event.PrimaryDeviceAttributesEvent:
		p.caps.primaryDeviceAttributes = append([]event.OptionalInt(nil), e...)
	case event.SecondaryDeviceAttributesEvent:
		p.caps.secondaryDeviceAttributes = append([]event.OptionalInt(nil), e...)
	case event.TertiaryDeviceAttributesEvent:
		p.caps.tertiaryDeviceAttributes, p.caps.hasTertiary = string(e), true
	case event.TerminalNameEvent:
		p.caps.terminalName, p.caps.hasTerminalName = string(e), true
	case event.KittyGraphicsEvent:
		p.caps.kittyGraphics = true
	case event.ProgramStatusEvent:
		p.caps.programStatus = &ansi.ProgramStatusCapabilities{
			States: slices.Clone(e.Capabilities.States),
			Kinds:  slices.Clone(e.Capabilities.Kinds),
		}
	case event.PaletteColorEvent:
		p.caps.palette[e.Index] = e.Color
	case event.ColorSchemeEvent:
		v := event.ColorScheme(e)
		p.caps.colorScheme = &v
	case event.ResizeEvent:
		p.cacheWindowSize(term.Winsize(e))
	case event.WindowCellSizeEvent:
		s := layout.Size{Width: e.Width, Height: e.Height}
		p.windowCells = &s
	case event.WindowPixelSizeEvent:
		s := layout.Size{Width: e.Width, Height: e.Height}
		p.windowPixels = &s
	case event.CellPixelSizeEvent:
		s := layout.Size{Width: e.Width, Height: e.Height}
		p.cellPixels = &s
	case event.CursorPositionEvent:
		if p.originQueriesPending > 0 {
			p.originQueriesPending--
			p.origin = p.clipOrigin(e.Position)
		}
	case event.TermcapEvent:
		for _, ent := range e.Entries {
			if e.Recognized {
				v := ent.Value
				if !ent.HasValue {
					v = ""
				}
				p.caps.termcap[ent.Name] = &v
			} else {
				p.caps.termcap[ent.Name] = nil
			}
		}
		if p.caps.SupportsTermcap("RGB") || p.caps.SupportsTermcap("Tc") {
			p.screen.SetColorProfile(vcolor.TrueColor)
		}
	}
	return nil
}

func (p *Program) cacheWindowSize(ws term.Winsize) {
	cells := layout.Size{Width: ws.Col, Height: ws.Row}
	p.windowCells = &cells
	if ws.XPixel > 0 && ws.YPixel > 0 {
		pixels := layout.Size{Width: ws.XPixel, Height: ws.YPixel}
		p.windowPixels = &pixels
	}
}

func (p *Program) clipOrigin(pos layout.Position) layout.Position {
	h := p.screen.Size().Height
	termH := h
	if p.windowCells != nil {
		termH = p.windowCells.Height
	}
	return layout.Pos(pos.X, min(pos.Y, max(termH-h, 0)))
}

func (p *Program) enableTabsAndBS() {
	p.screen.SetOptimizations(p.screen.Optimizations().WithTabs(true).WithBS(true))
}

func (p *Program) teardown() error {
	if err := p.reset(); err != nil {
		return err
	}
	return p.screen.Flush()
}

func writeString(w io.Writer, s string) error { _, err := io.WriteString(w, s); return err }
