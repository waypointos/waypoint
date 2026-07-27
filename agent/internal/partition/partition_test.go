package partition

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCmdline(t *testing.T, contents string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cmdline")
	if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestActive(t *testing.T) {
	cases := []struct {
		name    string
		cmdline string
		want    string
		wantErr bool
	}{
		{"slot A", "root=PARTUUID=" + AUUID + " rootfstype=squashfs ro\n", "A", false},
		{"slot B", "root=PARTUUID=" + BUUID + " rootfstype=squashfs ro\n", "B", false},
		{"uppercase PARTUUID still matches", "root=PARTUUID=BBBBBBBB-BBBB-BBBB-BBBB-BBBBBBBBBBBB ro\n", "B", false},
		{"unknown PARTUUID", "root=PARTUUID=12345678-1234-1234-1234-123456789abc ro\n", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Active(writeCmdline(t, tc.cmdline))
			if tc.wantErr != (err != nil) {
				t.Fatalf("Active() err = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("Active() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestActive_UnreadableFile(t *testing.T) {
	if _, err := Active(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("Active() on missing file: want error, got nil")
	}
}
