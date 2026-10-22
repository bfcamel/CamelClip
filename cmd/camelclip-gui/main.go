// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/bfcamel/CamelClip/internal/bootstrap"
	"github.com/bfcamel/CamelClip/internal/ui/gui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "CamelClip:", err)
		os.Exit(1)
	}
}
func run() error {
	browser := flag.Bool("browser", false, "open in the default browser instead of the desktop window")
	noOpen := flag.Bool("no-open", false, "print the private local URL without opening a window")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, "CamelClip")
	if v := os.Getenv("CAMELCLIP_CONFIG_DIR"); v != "" {
		dir = v
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	backend := bootstrap.Downloader()
	app, err := gui.New(ctx, gui.Dependencies{OpenConfig: bootstrap.OpenConfig, OpenWorkspace: bootstrap.OpenWorkspace(backend), OpenMedia: gui.OpenMedia}, filepath.Join(dir, "settings.json"), gui.Defaults(home), listener.Addr().String(), stop)
	if err != nil {
		return err
	}
	defer app.Close()
	server := &http.Server{Handler: app, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	url := app.URL()
	if *noOpen {
		fmt.Println(url)
	} else {
		exe, _ := os.Executable()
		host := filepath.Join(filepath.Dir(exe), "CamelClipWindow")
		if _, e := os.Stat(host); e == nil && !*browser {
			child := exec.CommandContext(ctx, host, url)
			child.Stderr = os.Stderr
			if err := child.Start(); err != nil {
				return err
			}
			go func() { child.Wait(); stop() }()
		} else if err := gui.OpenBrowser(ctx, url); err != nil {
			return err
		}
	}
	select {
	case <-ctx.Done():
		return nil
	case err := <-serveErr:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
