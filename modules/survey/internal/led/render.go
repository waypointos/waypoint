package led

// Pattern is the semantic strip state; Render turns it into pixels.
type Pattern struct {
	Base   RGB
	ID     int
	ShowID bool
}

var (
	Green  = RGB{G: 255}
	Red    = RGB{R: 255}
	Yellow = RGB{R: 255, G: 180}
	white  = RGB{R: 255, G: 255, B: 255}
)

// idBits is the binary display width: marker ids fit in 7 bits.
const idBits = 7

// Render produces the strip frame. Plain patterns fill the strip with the
// base color. With ShowID, LED 0 stays the base color as the state pixel and
// LEDs 1..7 show the id MSB-first, white for 1, off for 0.
func Render(p Pattern, count int, brightness float64) []RGB {
	out := make([]RGB, count)
	base := Scale(p.Base, brightness)
	if !p.ShowID {
		for i := range out {
			out[i] = base
		}
		return out
	}
	if count > 0 {
		out[0] = base
	}
	for i := 0; i < idBits && i+1 < count; i++ {
		if (p.ID>>(idBits-1-i))&1 == 1 {
			out[i+1] = Scale(white, brightness)
		}
	}
	return out
}
