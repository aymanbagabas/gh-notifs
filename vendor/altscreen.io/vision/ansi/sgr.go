package ansi

import "strconv"

// SGR returns a Select Graphic Rendition sequence: ESC [ params m.
func SGR(params [][]int) string {
	out := "\x1b["
	firstGroup := true
	for _, group := range params {
		if len(group) == 0 {
			continue
		}
		if !firstGroup {
			out += ";"
		}
		firstGroup = false
		for i, p := range group {
			if i > 0 {
				out += ":"
			}
			out += strconv.Itoa(p)
		}
	}
	return out + "m"
}
