package led

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 0x00 expands to eight 100 symbols, 0xFF to eight 110 symbols.
var (
	zeroByte = []byte{0x92, 0x49, 0x24}
	fullByte = []byte{0xDB, 0x6D, 0xB6}
)

func TestEncodeSingleRed(t *testing.T) {
	// Wire order is GRB: red = 0x00, 0xFF, 0x00.
	got := Encode([]RGB{{R: 255}})
	want := append(append(append([]byte{}, zeroByte...), fullByte...), zeroByte...)
	require.Len(t, got, 9+latchBytes)
	assert.Equal(t, want, got[:9])
	assert.Equal(t, bytes.Repeat([]byte{0}, latchBytes), got[9:])
}

func TestEncodeGreenFirstOnWire(t *testing.T) {
	got := Encode([]RGB{{G: 255}})
	assert.Equal(t, fullByte, got[:3])
	assert.Equal(t, zeroByte, got[3:6])
}

func TestEncodeKnownPattern(t *testing.T) {
	// 0xAA = 10101010 -> symbols 110 100 110 100 110 100 110 100.
	got := Encode([]RGB{{G: 0xAA}})
	assert.Equal(t, []byte{0b11010011, 0b01001101, 0b00110100}, got[:3])
}

func TestEncodeLength(t *testing.T) {
	assert.Len(t, Encode(make([]RGB, 8)), 8*9+latchBytes)
}

func TestScale(t *testing.T) {
	assert.Equal(t, RGB{R: 127, G: 50, B: 0}, Scale(RGB{R: 255, G: 100, B: 0}, 0.5))
	assert.Equal(t, RGB{}, Scale(RGB{R: 255}, 0))
	// Out-of-range brightness clamps instead of overflowing.
	assert.Equal(t, RGB{R: 255}, Scale(RGB{R: 255}, 2))
	assert.Equal(t, RGB{}, Scale(RGB{R: 255}, -1))
}
