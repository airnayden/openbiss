//go:build !linux

package gui

func hasStatusNotifierWatcher() bool {
	return true
}
