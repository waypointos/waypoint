//go:build !linux

package sysinfo

type noopProbes struct{}

func (noopProbes) CPUPercent() (float64, bool)         { return 0, false }
func (noopProbes) TempC() (float64, bool)              { return 0, false }
func (noopProbes) MemoryBytes() (uint64, uint64, bool) { return 0, 0, false }
func (noopProbes) UptimeSeconds() (uint64, bool)       { return 0, false }

func defaultProbes() Probes { return noopProbes{} }
