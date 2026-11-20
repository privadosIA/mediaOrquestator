//go:build !unix

package runner

import "os/exec"

// inOwnGroup does nothing where process groups do not exist: cancelling the context kills the program itself.
func inOwnGroup(*exec.Cmd) {}
