package color

// RGBToAnsi256 converts an RGB color to the nearest xterm 256-color index.
func RGBToAnsi256(c RGB) Indexed { return rgbToAnsi256(c) }

// RGBToAnsi16 converts an RGB color to the nearest named ANSI color.
func RGBToAnsi16(c RGB) Named { return rgbToAnsi16(c) }

func rgbToAnsi256(c RGB) Indexed {
	cube := [...]uint8{0, 0x5f, 0x87, 0xaf, 0xd7, 0xff}
	ri := nearestCubeIndex(c.R, cube)
	gi := nearestCubeIndex(c.G, cube)
	bi := nearestCubeIndex(c.B, cube)
	cubeIdx := uint8(16 + 36*ri + 6*gi + bi)
	cubeDist := colorDistanceSq(c, indexedToRGB(cubeIdx))

	avg := c.R
	if c.R != c.G || c.G != c.B {
		avg = uint8((uint16(c.R) + uint16(c.G) + uint16(c.B)) / 3)
	}
	grayIdx := nearestGray(avg)
	grayDist := colorDistanceSq(c, indexedToRGB(grayIdx))

	if grayDist < cubeDist {
		return Indexed(grayIdx)
	}
	return Indexed(cubeIdx)
}

func rgbToAnsi16(c RGB) Named {
	bestIdx := uint8(0)
	bestDist := ^uint32(0)
	for i := uint8(0); i < 16; i++ {
		d := colorDistanceSq(c, indexedToRGB(i))
		if d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	return Named(bestIdx)
}

func nearestCubeIndex(v uint8, cube [6]uint8) uint8 {
	best := uint8(0)
	bestDist := uint8(255)
	for i, cv := range cube {
		d := absDiff(v, cv)
		if d < bestDist {
			bestDist = d
			best = uint8(i)
		}
	}
	return best
}

func nearestGray(v uint8) uint8 {
	if v < 4 {
		return 232
	}
	if v > 243 {
		return 255
	}
	return uint8(min((uint16(v)-8+5)/10+232, 255))
}

func colorDistanceSq(a, b RGB) uint32 {
	dr := int32(a.R) - int32(b.R)
	dg := int32(a.G) - int32(b.G)
	db := int32(a.B) - int32(b.B)
	return uint32(2*dr*dr + 4*dg*dg + 3*db*db)
}

func absDiff(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}
