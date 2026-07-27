package cameras

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fanout flushes an access unit when the next AUD (NAL type 9) arrives.
// Synthetic NALs: 0x65 = IDR slice (keyframe), 0x41 = non-IDR slice.
func TestCameraTapReceivesAccessUnits(t *testing.T) {
	c := &Camera{}
	type got struct {
		au []byte
		kf bool
	}
	rx := make(chan got, 4)
	remove := c.AddTap(func(au []byte, keyframe bool) {
		cp := make([]byte, len(au))
		copy(cp, au)
		rx <- got{cp, keyframe}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	frames := make(chan []byte, 8)
	go c.fanout(ctx, frames)

	frames <- []byte{0x09, 0x10} // AUD opens AU 1
	frames <- []byte{0x65, 0xAA} // IDR slice
	frames <- []byte{0x09, 0x10} // AUD: flushes AU 1, opens AU 2
	frames <- []byte{0x41, 0xBB} // non-IDR slice
	frames <- []byte{0x09, 0x10} // flushes AU 2

	first := <-rx
	require.True(t, first.kf, "AU containing an IDR NAL must flag keyframe")
	require.Contains(t, string(first.au), string([]byte{0x00, 0x00, 0x00, 0x01, 0x65}))
	second := <-rx
	require.False(t, second.kf)

	remove()
	frames <- []byte{0x65, 0xCC}
	frames <- []byte{0x09, 0x10}
	select {
	case <-rx:
		t.Fatal("tap fired after remove()")
	case <-time.After(100 * time.Millisecond):
	}
}
