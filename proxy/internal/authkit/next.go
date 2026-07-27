package authkit

import "strings"

// SanitizeNext returns a safe relative path to redirect to after sign-in.
// Anything that could redirect off-origin, parse oddly, or loop back through
// the auth flow falls back to "/".
func SanitizeNext(s string) string {
	if s == "" {
		return "/"
	}
	if !strings.HasPrefix(s, "/") {
		return "/"
	}
	if strings.HasPrefix(s, "//") {
		return "/"
	}
	if strings.Contains(s, "\\") {
		return "/"
	}
	// Reject control characters that could enable CRLF injection in the
	// Location header (browsers and proxies treat \r\n as header separators).
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return "/"
		}
	}
	// Reject URL-encoded backslash. Go decodes %XX before we get here in the
	// typical query-string path, but a manually-built input could still
	// contain encoded bytes.
	if strings.Contains(s, "%5C") || strings.Contains(s, "%5c") {
		return "/"
	}
	// Loop prevention: never bounce to a path that immediately re-enters auth.
	loops := []string{"/auth/login", "/auth/callback", "/auth/logout"}
	for _, p := range loops {
		if s == p || strings.HasPrefix(s, p+"?") || strings.HasPrefix(s, p+"/") {
			return "/"
		}
	}
	return s
}
