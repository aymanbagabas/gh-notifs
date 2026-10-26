// Package layout provides the geometry primitives for the cell grid.
//
// The origin is the top-left corner of the grid. X increases to the right and
// Y increases downward, so (0, 0) is the first cell and a larger Y is further
// down the screen. A [Rectangle] covers the half-open ranges Min.X..Max.X
// (columns) and Min.Y..Max.Y (rows):
//
//	    x:  0   1   2   3   4
//	      ┌───┬───┬───┬───┬───┐
//	y: 0  │   │   │   │   │   │
//	      ├───┼───┼───┼───┼───┤
//	   1  │   │ ▓ │ ▓ │   │   │   Rect(1, 1, 2, 2)
//	      ├───┼───┼───┼───┼───┤   origin (1, 1), 2 × 2 cells,
//	   2  │   │ ▓ │ ▓ │   │   │   covering x ∈ 1..3, y ∈ 1..3
//	      ├───┼───┼───┼───┼───┤
//	   3  │   │   │   │   │   │
//	      └───┴───┴───┴───┴───┘
//
// [Position] and [Rectangle] are aliases for [image.Point] and
// [image.Rectangle], so the geometry helpers in the standard library apply
// directly: r.Empty, r.Intersect, r.Union, r.Dx, r.Dy, and p.In(r) all work as
// they do for images.
package layout

import (
	"image"
	"strconv"
)

// Position is a point in the cell grid: X is the column, Y is the row. Both
// are zero-based.
type Position = image.Point

// Pos returns the [Position] at column x, row y.
func Pos(x, y int) Position {
	return image.Point{X: x, Y: y}
}

// Rectangle is an axis-aligned rectangle in the cell grid. It is anchored at
// Min and its Max edges are exclusive, so a cell at p is contained iff
// p.In(r).
type Rectangle = image.Rectangle

// Rect returns the [Rectangle] anchored at (x, y) that extends width cells to
// the right and height cells down.
func Rect(x, y, width, height int) Rectangle {
	return Rectangle{
		Min: image.Point{X: x, Y: y},
		Max: image.Point{X: x + width, Y: y + height},
	}
}

// Area returns the number of cells r covers. It is zero for an empty
// rectangle, including one whose edges are inverted.
func Area(r Rectangle) int {
	if r.Empty() {
		return 0
	}
	return r.Dx() * r.Dy()
}

// Size is an extent in the cell grid: Width columns by Height rows.
type Size struct {
	// Width is the extent in cells (columns).
	Width int
	// Height is the extent in cells (rows).
	Height int
}

// SizeOf returns the extent of r, dropping its position.
func SizeOf(r Rectangle) Size {
	return Size{Width: r.Dx(), Height: r.Dy()}
}

// String returns the size as "WIDTHxHEIGHT".
func (s Size) String() string {
	return strconv.Itoa(s.Width) + "x" + strconv.Itoa(s.Height)
}
