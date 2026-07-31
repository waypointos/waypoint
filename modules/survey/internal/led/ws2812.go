// Package led drives a WS2812 strip over SPI by expanding each WS2812 bit
// into 3 SPI bits at 2.4 MHz, and renders the mission's LED patterns.
package led

type RGB struct {
	R uint8
	G uint8
	B uint8
}

// SPISpeedHz is 3x the WS2812 800 kHz bit rate: each data bit becomes a
// 3-bit SPI symbol (1 -> 110, 0 -> 100) so pulse widths land in spec.
const SPISpeedHz = 2_400_000

// latchBytes of zeros hold the line low >50us so the strip latches.
const latchBytes = 64

// Encode expands pixels into the SPI byte stream, GRB wire order, plus the
// trailing latch gap.
func Encode(pixels []RGB) []byte {
	out := make([]byte, 0, len(pixels)*9+latchBytes)
	var acc uint32
	var nbits uint
	putSymbol := func(bit uint8) {
		sym := uint32(0b100)
		if bit != 0 {
			sym = 0b110
		}
		acc = acc<<3 | sym
		nbits += 3
		for nbits >= 8 {
			nbits -= 8
			out = append(out, byte(acc>>nbits))
		}
	}
	for _, px := range pixels {
		for _, ch := range [3]uint8{px.G, px.R, px.B} {
			for i := 7; i >= 0; i-- {
				putSymbol((ch >> uint(i)) & 1)
			}
		}
	}
	// 24 ws-bits per pixel yield whole bytes (72 spi bits), so acc is empty.
	for i := 0; i < latchBytes; i++ {
		out = append(out, 0)
	}
	return out
}

// Scale dims a color by brightness in [0,1].
func Scale(c RGB, brightness float64) RGB {
	if brightness < 0 {
		brightness = 0
	}
	if brightness > 1 {
		brightness = 1
	}
	return RGB{
		R: uint8(float64(c.R) * brightness),
		G: uint8(float64(c.G) * brightness),
		B: uint8(float64(c.B) * brightness),
	}
}
