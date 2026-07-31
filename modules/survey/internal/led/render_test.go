package led

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderSolid(t *testing.T) {
	got := Render(Pattern{Base: Green}, 8, 1.0)
	require.Len(t, got, 8)
	for _, px := range got {
		assert.Equal(t, Green, px)
	}
}

func TestRenderBinaryID(t *testing.T) {
	// 17 = 0010001: LED 0 is the state pixel, the MSB starts at LED 1,
	// so LEDs 3 and 7 are white and unset bits are off.
	got := Render(Pattern{Base: Yellow, ID: 17, ShowID: true}, 8, 1.0)
	require.Len(t, got, 8)
	whitePx := RGB{R: 255, G: 255, B: 255}
	assert.Equal(t, Yellow, got[0], "state pixel")
	for i := 1; i < 8; i++ {
		if i == 3 || i == 7 {
			assert.Equal(t, whitePx, got[i], "led %d", i)
		} else {
			assert.Equal(t, RGB{}, got[i], "led %d", i)
		}
	}
}

func TestRenderBinaryAllBits(t *testing.T) {
	got := Render(Pattern{Base: Yellow, ID: 0x7F, ShowID: true}, 8, 1.0)
	whitePx := RGB{R: 255, G: 255, B: 255}
	assert.Equal(t, Yellow, got[0], "state pixel")
	for i := 1; i < 8; i++ {
		assert.Equal(t, whitePx, got[i], "led %d", i)
	}
}

func TestRenderBrightnessApplies(t *testing.T) {
	got := Render(Pattern{Base: Red}, 4, 0.4)
	assert.Equal(t, Scale(Red, 0.4), got[0])
}

func TestRenderShortStrip(t *testing.T) {
	// A strip shorter than the id field truncates the display safely.
	got := Render(Pattern{Base: Yellow, ID: 0x7F, ShowID: true}, 4, 1.0)
	require.Len(t, got, 4)
	assert.Equal(t, Yellow, got[0])
}
