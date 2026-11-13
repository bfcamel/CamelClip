#!/bin/sh
# Package preinstall runs as root. Probe and provision tools as the desktop user.
set -eu
user=$(/usr/bin/stat -f '%Su' /dev/console)
case "$user" in root|loginwindow|'') echo 'Use the user setup archive from a signed-in account.' >&2; exit 1;; esac
uid=$(/usr/bin/id -u "$user")
dir=$(/usr/bin/mktemp -d /private/tmp/camelclip-dependencies.XXXXXX)
trap '/bin/rm -rf "$dir"' EXIT
/bin/chmod 755 "$dir"
/usr/bin/install -m 755 "$1" "$dir/setup"
as_user() {
 /bin/launchctl asuser "$uid" /usr/bin/sudo -H -u "$user" "$@"
}
if as_user "$dir/setup" --dependencies check; then exit 0; fi
# A real terminal lets Homebrew request sudo or developer tools when needed.
as_user /usr/bin/osascript - "$dir/setup" <<'APPLESCRIPT'
on run argv
 set exe to quoted form of item 1 of argv
 tell application "Terminal"
  activate
  set setupTab to do script exe & " --dependencies plan && " & exe & " --dependencies install"
  delay 1
  repeat while busy of setupTab
   delay 1
  end repeat
 end tell
end run
APPLESCRIPT
as_user "$dir/setup" --dependencies check
