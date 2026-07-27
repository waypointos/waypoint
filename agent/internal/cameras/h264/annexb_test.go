package h264

import (
	"bytes"
	"testing"
)

func TestSplit_TwoNALs_3ByteStart(t *testing.T) {
	in := []byte{
		0, 0, 1, 0x67, 0x42, 0x00, 0x1e, // SPS
		0, 0, 1, 0x68, 0xce, 0x06, 0xe2, // PPS
	}
	r := NewReader(bytes.NewReader(in))
	got := drain(t, r)
	want := [][]byte{
		{0x67, 0x42, 0x00, 0x1e},
		{0x68, 0xce, 0x06, 0xe2},
	}
	assertEq(t, got, want)
}

func TestSplit_4ByteStart(t *testing.T) {
	in := []byte{
		0, 0, 0, 1, 0x67, 0x42,
		0, 0, 0, 1, 0x65, 0x88,
	}
	got := drain(t, NewReader(bytes.NewReader(in)))
	want := [][]byte{
		{0x67, 0x42}, {0x65, 0x88},
	}
	assertEq(t, got, want)
}

func TestSplit_MixedStartLengths(t *testing.T) {
	in := []byte{
		0, 0, 0, 1, 0x67, 0x42,
		0, 0, 1, 0x68, 0xce,
		0, 0, 0, 1, 0x65, 0x88,
	}
	got := drain(t, NewReader(bytes.NewReader(in)))
	want := [][]byte{
		{0x67, 0x42}, {0x68, 0xce}, {0x65, 0x88},
	}
	assertEq(t, got, want)
}

func TestSplit_EmptyTrailingTail(t *testing.T) {
	// real-world: pipeline produces a long stream; ReadNAL must not block
	// waiting for a non-existent terminator. We assert: the reader returns
	// the last NAL on EOF, not a hung goroutine.
	in := []byte{0, 0, 1, 0x67, 0x42, 0x00, 0x1e}
	r := NewReader(bytes.NewReader(in))
	nal, err := r.ReadNAL()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(nal, []byte{0x67, 0x42, 0x00, 0x1e}) {
		t.Fatalf("got %x", nal)
	}
	if _, err := r.ReadNAL(); err == nil {
		t.Fatal("expected io.EOF after last NAL")
	}
}

func drain(t *testing.T, r *Reader) [][]byte {
	t.Helper()
	var out [][]byte
	for {
		n, err := r.ReadNAL()
		if err != nil {
			return out
		}
		out = append(out, n)
	}
}

func assertEq(t *testing.T, got, want [][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("NAL %d: got %x want %x", i, got[i], want[i])
		}
	}
}
