package tools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLinuxDependencyRoutes(t *testing.T) {
	for _, route := range []string{"present", "apt", "fallback", "bootstrap"} {
		t.Run(route, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range []string{"sed", "sort", "tr", "awk", "mktemp", "rm"} {
				src, err := exec.LookPath(name)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.Symlink(src, filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			}
			tool := func(name, body string) {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			tool("uname", "echo Linux")
			tool("id", "echo 1000")
			tool("sudo", "exec \"$@\"")
			for _, name := range []string{"ffmpeg", "ffprobe", "yt-dlp"} {
				tool(name, "exit 0")
			}
			tool("deno", `if [ -f "$TOOLS/ready" ]; then echo 'deno 2.3.0';else echo 'deno 1.4.0';fi`)
			if route == "present" {
				os.WriteFile(filepath.Join(dir, "ready"), nil, 0600)
			}
			if route != "bootstrap" {
				tool("apt-cache", `if [ "$ROUTE" = apt ];then echo '  Candidate: 2.3';else echo '  Candidate: (none)';fi`)
				tool("apt-get", `echo "apt $*" >> "$TOOLS/log"; if [ "$1" = install ];then : > "$TOOLS/ready";fi`)
			}
			brew := `echo "brew $*" >> "$TOOLS/log"; : > "$TOOLS/ready"`
			if route != "bootstrap" {
				tool("brew", brew)
			} else {
				tool("curl", `while [ "$1" != -o ];do shift;done
printf '%s\n' '/bin/cp "$TOOLS/brew-fixture" "$TOOLS/brew"' > "$2"`)
				tool("brew-fixture", brew)
			}
			fixtureScript := strings.ReplaceAll(script, "/home/linuxbrew/.linuxbrew/bin", dir)
			cmd := exec.Command("/bin/sh", "-c", fixtureScript, "setup", "install")
			cmd.Env = append(os.Environ(), "TOOLS="+dir, "PATH="+dir, "ROUTE="+route)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			log, _ := os.ReadFile(filepath.Join(dir, "log"))
			text := string(log)
			switch route {
			case "present":
				if text != "" {
					t.Fatal(text)
				}
			case "apt":
				if !strings.Contains(text, "apt install -y deno") || strings.Contains(text, "brew") {
					t.Fatal(text)
				}
			default:
				if !strings.Contains(text, "brew install deno") {
					t.Fatal(text)
				}
			}
		})
	}
}
