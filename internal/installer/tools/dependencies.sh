# Executed by CamelClip Setup as the installing user.
set -eu
mode=$1
system=$(uname -s)
case "$system" in
 Darwin) PATH="$PATH:/opt/homebrew/bin:/usr/local/bin";;
 Linux) PATH="$PATH:/home/linuxbrew/.linuxbrew/bin";;
esac
export PATH
missing() {
 for tool in yt-dlp ffmpeg ffprobe deno; do
  case "$tool" in ffmpeg|ffprobe) arg=-version;; *) arg=--version;; esac
  if ! version=$("$tool" "$arg" 2>/dev/null); then echo "$tool"; continue; fi
  if [ "$tool" = deno ] && ! printf '%s\n' "$version" | awk 'NR==1 {split($2,v,"."); ok=($1=="deno" && (v[1]+0>2 || (v[1]+0==2 && v[2]+0>=3)))} END {exit !ok}'; then echo deno; fi
 done
}
packages() {
 printf '%s\n' "$@" | sed 's/^ffprobe$/ffmpeg/' | sort -u
}
set -- $(missing)
[ "$#" -gt 0 ] || exit 0
manager=""
if [ "$system" = Linux ]; then
 for m in apt-get dnf pacman; do
  if command -v "$m" >/dev/null; then manager=$m; break; fi
 done
fi
echo "Required tools missing or unable to run: $*"
echo "Dependency plan: ${manager:-Homebrew} for $(packages "$@" | tr '\n' ' ')."
if [ -n "$manager" ]; then echo "Homebrew will supply packages unavailable from $manager."; fi
if ! command -v brew >/dev/null; then echo "If Homebrew is needed, setup will install it first."; fi
case "$mode" in plan) exit 0;; check) exit 1;; install) ;; *) exit 2;; esac
admin() {
 if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi
}
if [ -n "$manager" ]; then
 case "$manager" in apt-get) admin apt-get update;; esac
 for pkg in $(packages "$@"); do
  case "$manager" in
   apt-get) if apt-cache policy "$pkg" | awk '/Candidate:/ && $2 != "(none)" {found=1} END {exit !found}'; then admin apt-get install -y "$pkg"; fi;;
   dnf) if dnf -q list "$pkg" >/dev/null 2>&1; then admin dnf install -y "$pkg"; fi;;
   pacman) if pacman -Si "$pkg" >/dev/null 2>&1; then admin pacman -S --needed --noconfirm "$pkg"; fi;;
  esac
 done
fi
set -- $(missing)
[ "$#" -gt 0 ] || exit 0
if ! command -v brew >/dev/null; then
 if [ "$(id -u)" -eq 0 ]; then echo "Homebrew requires a non-root account. Install $* manually or rerun setup as the desktop user." >&2; exit 1; fi
 bootstrap=$(mktemp)
 trap 'rm -f "$bootstrap"' EXIT
 curl --fail --show-error --silent --location --proto '=https' https://raw.githubusercontent.com/Homebrew/install/e5b3c5834e3932c8aaa42deef4794076c9b0cbf0/install.sh -o "$bootstrap"
 if [ -t 0 ] && command -v sudo >/dev/null; then sudo -v; fi
 NONINTERACTIVE=1 /bin/bash "$bootstrap" || { echo "Homebrew setup failed. Install $* manually, then rerun CamelClip Setup." >&2; exit 1; }
fi
brew install $(packages "$@")
remaining=$(missing)
[ -z "$remaining" ] || { echo "Required tools still unavailable: $remaining. Check PATH and rerun setup." >&2; exit 1; }
