// Copyright 2026 bfcamel and contributors.
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bfcamel/CamelClip/internal/installer"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "CamelClip Setup:", e)
		os.Exit(1)
	}
}
func run() error {
	flags := flag.NewFlagSet("camelclip-setup", flag.ContinueOnError)
	components := flags.String("components", "", "components to add/update: gui,tui,cli; installed choices are preserved")
	remove := flags.String("remove", "", "components to remove: gui,tui,cli")
	uninstall := flags.Bool("uninstall", false, "remove all managed components; retain settings and media")
	preflight := flags.String("preflight-record", "", "validate incoming macOS package ownership (internal)")
	status := flags.Bool("status", false, "show installed components")
	yes := flags.Bool("yes", false, "confirm the displayed installation/removal plan")
	prefix := flags.String("prefix", "", "user installation prefix (default ~/.local)")
	scope := flags.String("scope", "user", "user or system (macOS package status/removal only)")
	source := flags.String("source", "", "directory containing manifest.json and payload (default executable directory)")
	configDir := flags.String("config-dir", "", "workspace config directory for the active-session check")
	require := flags.Bool("require-dependencies", false, "stop if required media tools are unavailable")
	if e := flags.Parse(os.Args[1:]); e != nil {
		return e
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if (*components != "" && (*remove != "" || *uninstall)) || (*remove != "" && *uninstall) {
		return fmt.Errorf("choose installation, removal, or uninstall")
	}
	var layout installer.Layout
	var e error
	if *scope == "system" {
		if *prefix != "" {
			return fmt.Errorf("--prefix is only available with --scope user")
		}
		layout, e = installer.SystemLayout()
	} else if *scope == "user" {
		layout, e = installer.UserLayout(*prefix)
	} else {
		return fmt.Errorf("scope must be user or system")
	}
	if e != nil {
		return e
	}
	if *preflight != "" {
		if *scope != "system" {
			return fmt.Errorf("package preflight requires system scope")
		}
		return installer.PreflightRecord(layout, *preflight)
	}
	current, e := installer.ReadInstalled(layout)
	if e != nil {
		return e
	}
	if *status {
		fmt.Printf("CamelClip (%s, %s/%s)\n", *scope, runtime.GOOS, runtime.GOARCH)
		for _, id := range installer.Visible(current) {
			fmt.Printf("  %-4s %s\n", id, current[id].Version)
		}
		fmt.Println("Installation:", layout.Data)
		return nil
	}
	in := bufio.NewReader(os.Stdin)
	if *components == "" && *remove == "" && !*uninstall {
		info, _ := os.Stdin.Stat()
		if info == nil || info.Mode()&os.ModeCharDevice == 0 {
			return fmt.Errorf("noninteractive setup requires --components, --remove, or --uninstall (and --yes)")
		}
		fmt.Println("CamelClip Setup / Установка CamelClip")
		fmt.Println("1  GUI — graphical application / графический интерфейс")
		fmt.Println("2  TUI + CLI — terminal workspace and commands / терминал и команды")
		fmt.Println("3  GUI + TUI + CLI — everything / всё")
		fmt.Println("4  Custom / свой выбор: gui,tui,cli")
		fmt.Println("5  Update installed components / обновить установленные")
		fmt.Println("6  Remove components / удалить компоненты")
		fmt.Println("0  Cancel / отмена")
		fmt.Print("> ")
		choice, _ := in.ReadString('\n')
		switch strings.TrimSpace(choice) {
		case "1":
			*components = "gui"
		case "2":
			*components = "tui,cli"
		case "3":
			*components = "gui,tui,cli"
		case "4":
			fmt.Print("gui,tui,cli > ")
			v, _ := in.ReadString('\n')
			*components = strings.TrimSpace(v)
		case "5":
			*components = strings.Join(installer.Visible(current), ",")
			if *components == "" {
				return fmt.Errorf("nothing installed yet")
			}
		case "6":
			fmt.Print("Remove / удалить (gui,tui,cli or all) > ")
			v, _ := in.ReadString('\n')
			v = strings.TrimSpace(v)
			if v == "all" {
				*uninstall = true
			} else {
				*remove = v
			}
		case "0":
			return nil
		default:
			return fmt.Errorf("unknown selection")
		}
	}
	if *scope == "system" && *components != "" {
		return fmt.Errorf("use the macOS .pkg installer for system installation/updates")
	}
	var add, del []string
	if *components != "" {
		add, e = installer.Choices(*components)
		if e != nil {
			return e
		}
	}
	if *remove != "" {
		del, e = installer.Choices(*remove)
		if e != nil {
			return e
		}
	}
	if *source == "" {
		exe, e := os.Executable()
		if e != nil {
			return e
		}
		*source = filepath.Dir(exe)
	}
	if len(add) > 0 {
		if _, e = installer.Load(*source); e != nil {
			return e
		}
		missing := []string{}
		for _, name := range []string{"yt-dlp", "ffmpeg", "ffprobe", "deno"} {
			if _, e := exec.LookPath(name); e != nil {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			fmt.Println("Missing tools / Не найдены:", strings.Join(missing, ", "))
			fmt.Println("Install these with your system package manager. Deno is recommended for YouTube. Setup does not install third-party tools.")
			if *require {
				return fmt.Errorf("dependency check failed")
			}
		}
	}
	if *configDir == "" {
		*configDir = os.Getenv("CAMELCLIP_CONFIG_DIR")
		if *configDir == "" {
			v, e := os.UserConfigDir()
			if e != nil {
				return e
			}
			*configDir = filepath.Join(v, "CamelClip")
		}
	}
	fmt.Printf("Scope: %s\nApplications: %s\nCommands: %s\n", *scope, layout.Apps, filepath.Join(layout.Prefix, "bin"))
	fmt.Println("Installed:", strings.Join(installer.Visible(current), ","))
	if len(add) > 0 {
		fmt.Println("Add/update:", strings.Join(add, ","), "(other installed components will also update)")
	}
	if len(del) > 0 {
		fmt.Println("Remove:", strings.Join(del, ","))
	}
	if *uninstall {
		fmt.Println("Remove all managed application files")
	}
	fmt.Println("Settings, download history, encrypted profiles and media will be retained.")
	if !*yes {
		fmt.Print("Continue / Продолжить? [y/N] ")
		v, _ := in.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(v)) != "y" {
			return fmt.Errorf("canceled; use --yes for unattended setup")
		}
	}
	if e = installer.Apply(layout, installer.Options{Source: *source, Components: add, Remove: del, Uninstall: *uninstall, ConfigDir: *configDir}); e != nil {
		return e
	}
	fmt.Println("Done / Готово.")
	if len(add) > 0 {
		fmt.Println("Launch:", filepath.Join(layout.Prefix, "bin"), "— camelclip-gui / camelclip-tui / camelclip (according to selected components). Ensure this bin directory is in PATH.")
	}
	return nil
}
