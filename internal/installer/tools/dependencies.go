// Package tools provisions external media tools on the installer's Unix host.
package tools

import (
	_ "embed"
	"os"
	"os/exec"
)

//go:embed dependencies.sh
var script string

func Dependencies(mode string) error {
	cmd := exec.Command("/bin/sh", "-c", script, "camelclip-dependencies", mode)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
