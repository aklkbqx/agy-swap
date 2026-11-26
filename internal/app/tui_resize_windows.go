//go:build windows

package app

func watchTerminalResize() (<-chan struct{}, func()) {
	return nil, func() {}
}
