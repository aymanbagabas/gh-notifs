package buffer

// Line is an owned row of cells.
type Line []Cell

func fillLineInto(line []Cell, fill *Cell) {
	f := cellOrBlank(fill)
	if f.Width() <= 1 {
		for i := range line {
			line[i] = f
		}
		return
	}
	for x := 0; x < len(line); {
		if f.Width() > len(line)-x {
			line[x] = BlankCell()
			x++
			continue
		}
		line[x] = f
		for i := 1; i < f.Width(); i++ {
			line[x+i] = ContinuationCell().WithStyle(f.Style)
		}
		x += f.Width()
	}
}

// FillLine returns a line of width cells filled with fill.
func FillLine(width int, fill *Cell) Line {
	line := make([]Cell, max(width, 0))
	fillLineInto(line, fill)
	return line
}

// BlankLine returns a blank line of width cells.
func BlankLine(width int) Line {
	blankCell := BlankCell()
	return FillLine(width, &blankCell)
}
