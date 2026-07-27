//go:build !linux

package clockanchor

// NTPSynced is Linux-only; other hosts report false (treated as unsynced).
func NTPSynced() bool { return false }
