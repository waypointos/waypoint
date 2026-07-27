//go:build darwin

package pipeline

func newMacOrErr(cfg Config) (Pipeline, error) { return NewMac(cfg), nil }
func newPi5OrErr(cfg Config) (Pipeline, error) { return nil, errNotSupported("pi5", "darwin") }
