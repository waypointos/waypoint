//go:build !(linux && arm64) && !darwin

package pipeline

func newMacOrErr(cfg Config) (Pipeline, error) { return nil, errNotSupported("mac", "this OS") }
func newPi5OrErr(cfg Config) (Pipeline, error) { return nil, errNotSupported("pi5", "this OS") }
