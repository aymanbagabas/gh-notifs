package ansi

import "strconv"

// InBandResize encodes an in-band resize report as ESC [ 48 ; cells ; pixels t.
func InBandResize(heightCells, widthCells, heightPixels, widthPixels int) string {
	return "\x1b[48;" + strconv.Itoa(heightCells) + ";" + strconv.Itoa(widthCells) + ";" + strconv.Itoa(heightPixels) + ";" + strconv.Itoa(widthPixels) + "t"
}
