package authkit

import "testing"

func TestSanitizeNext(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "/"},
		{"/", "/"},
		{"/rover/abc", "/rover/abc"},
		{"/admin/users", "/admin/users"},
		{"//evil.com/x", "/"},     // protocol-relative
		{"///evil.com", "/"},
		{"http://evil.com", "/"},  // absolute URL
		{"https://evil.com", "/"},
		{"javascript:alert(1)", "/"},
		{"\\evil", "/"},           // backslash trick
		{"/path\\with\\backslash", "/"},
		{"/\x00null", "/"},
		{"/\rcarriage", "/"},
		{"/\nnewline", "/"},
		{"/\r\nSet-Cookie: x=y", "/"},
		{"/\ttab", "/"},
		{"/\x7fdel", "/"},
		{"/%5Cevil", "/"},
		{"/%5cevil", "/"},
		{"/path%5Cwith%5Cencoded", "/"},
		{"/auth/login", "/"},      // loop prevention
		{"/auth/callback", "/"},
		{"/auth/logout", "/"},
		{"relative", "/"},         // no leading slash
		{"#fragment", "/"},
		{"?query=only", "/"},
		{"/rover/abc?tab=video", "/rover/abc?tab=video"}, // query preserved
		{"/rover/abc#section", "/rover/abc#section"},     // fragment preserved
	}
	for _, c := range cases {
		got := SanitizeNext(c.in)
		if got != c.want {
			t.Errorf("SanitizeNext(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
