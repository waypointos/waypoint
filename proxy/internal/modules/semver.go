package modules

import "golang.org/x/mod/semver"

// IsNewerVersion reports whether candidate is strictly greater (semver) than
// every version in existing. "v" prefix is optional. existing may be empty.
func IsNewerVersion(candidate string, existing []string) bool {
	c := canon(candidate)
	if !semver.IsValid(c) {
		return false
	}
	for _, e := range existing {
		if semver.Compare(c, canon(e)) <= 0 {
			return false
		}
	}
	return true
}

func canon(v string) string {
	if len(v) == 0 || v[0] != 'v' {
		return "v" + v
	}
	return v
}
