// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0

// Package wsl connects the Linux frontends to the Windows user's desktop.
package wsl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func Active() bool {
	return runtime.GOOS == "linux" && (os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "")
}

// PowerShell receives paths and URLs as data, including quotes and shell metacharacters.
func powershell(ctx context.Context, script, target string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command",
		"$ErrorActionPreference='Stop'; [Console]::InputEncoding=[Text.UTF8Encoding]::new($false); [Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); "+script)
	cmd.Stdin = strings.NewReader(target)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("Windows desktop integration: %w", err)
	}
	return out, nil
}

func Downloads() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := powershell(ctx, "(New-Object -ComObject Shell.Application).NameSpace('shell:Downloads').Self.Path", "")
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(raw))
	if path == "" {
		return ""
	}
	out, err := exec.CommandContext(ctx, "wslpath", "-u", path).Output()
	dir := strings.TrimSuffix(string(out), "\n")
	if err != nil || !filepath.IsAbs(dir) {
		return ""
	}
	return dir
}

func OpenBrowser(ctx context.Context, url string) error {
	_, err := powershell(ctx, "Start-Process -FilePath ([Console]::In.ReadToEnd())", url)
	return err
}

func OpenMedia(ctx context.Context, path string, reveal bool) error {
	out, err := exec.CommandContext(ctx, "wslpath", "-w", path).Output()
	if err != nil {
		return fmt.Errorf("convert path for Windows: %w", err)
	}
	target := strings.TrimRight(string(out), "\r\n")
	if target == "" {
		return errors.New("empty Windows path")
	}
	// Open the associated application or select the exact item in Explorer.
	script := "$path=[Console]::In.ReadToEnd(); Invoke-Item -LiteralPath $path"
	if reveal {
		script = `$path=[Console]::In.ReadToEnd(); Start-Process explorer.exe -ArgumentList ('/select,"'+$path+'"')`
	}
	_, err = powershell(ctx, script, target)
	return err
}
