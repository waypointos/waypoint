package cameras

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAssembleSpecs_SortsByDeviceIndexAndNames(t *testing.T) {
	// Names track the /dev/videoN index (not list position), and are ordered
	// by that index — so video0 → camera-0, video2 → camera-2.
	got := assembleSpecs([]string{"/dev/video2", "/dev/video0"})
	assert.Equal(t, []CameraSpec{
		{Name: "camera-0", Device: "/dev/video0", Pipeline: ""},
		{Name: "camera-2", Device: "/dev/video2", Pipeline: ""},
	}, got)
}

func TestAssembleSpecs_NumericNotLexicalOrder(t *testing.T) {
	// Lexical sort would put video10 before video2; we want numeric order.
	got := assembleSpecs([]string{"/dev/video10", "/dev/video2"})
	assert.Equal(t, "/dev/video2", got[0].Device)
	assert.Equal(t, "camera-2", got[0].Name)
	assert.Equal(t, "/dev/video10", got[1].Device)
	assert.Equal(t, "camera-10", got[1].Name)
}

func TestAssembleSpecs_Empty(t *testing.T) {
	assert.Empty(t, assembleSpecs(nil))
}

func TestAssembleSpecs_Single(t *testing.T) {
	got := assembleSpecs([]string{"/dev/video0"})
	assert.Equal(t, []CameraSpec{{Name: "camera-0", Device: "/dev/video0", Pipeline: ""}}, got)
}
