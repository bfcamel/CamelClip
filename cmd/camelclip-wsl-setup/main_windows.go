// Windows console bootstrap for the matching Linux release archive.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

//go:embed setup.ps1
var script string
var version, digest string

func main() {
	dir, err := os.MkdirTemp("", "camelclip-wsl-")
	if err == nil {
		defer os.RemoveAll(dir)
		path := filepath.Join(dir, "setup.ps1")
		err = os.WriteFile(path, []byte(script), 0600)
		if err == nil {
			exe, e := os.Executable()
			if e != nil {
				err = e
			} else {
				cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", path, "-VersionTag", version, "-Arch", runtime.GOARCH, "-Digest", digest, "-Launcher", exe)
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				err = cmd.Run()
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Setup failed:", err)
		fmt.Print("Press Enter to close.")
		fmt.Scanln()
		os.Exit(1)
	}
}
