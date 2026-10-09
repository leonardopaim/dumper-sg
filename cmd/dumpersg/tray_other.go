//go:build !windows

package main

func startTray(_ string, _ func() error) (func(), error) {
	return func() {}, nil
}
