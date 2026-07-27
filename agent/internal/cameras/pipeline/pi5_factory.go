//go:build linux && arm64

package pipeline

func newMacOrErr(cfg Config) (Pipeline, error) { return nil, errNotSupported("mac", "linux/arm64") }
func newPi5OrErr(cfg Config) (Pipeline, error) { return NewPi5(cfg), nil }
