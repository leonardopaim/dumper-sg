//go:build !windows

package docker

import "os/exec"

func hideConsole(*exec.Cmd) {}
