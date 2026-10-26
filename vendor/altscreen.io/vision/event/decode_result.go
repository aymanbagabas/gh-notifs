package event

type parseResult struct {
	event      Event
	consumed   int
	incomplete bool
	none       bool
}

func resultEvent(ev Event, n int) parseResult { return parseResult{event: ev, consumed: n} }
func resultIncomplete() parseResult           { return parseResult{incomplete: true} }
func resultNone(n int) parseResult            { return parseResult{none: true, consumed: n} }
