#!/usr/bin/env bash
# shellcheck disable=SC2015 # ok/warn always succeed, so A && ok || bad is safe here
# Installs, checks or removes the scheduled native session on a Mac (FLOW-09).
#
#   tools/workflow/native/install-macos.sh check     [machine]   # report only; changes nothing
#   tools/workflow/native/install-macos.sh install   [machine] [HH:MM]
#   tools/workflow/native/install-macos.sh uninstall
#
# machine defaults to macbook (hosts.yaml on the workflow branch); HH:MM is
# local time and defaults to 04:40, after the cloud tick at 02:55 Pacific. Run
# it from the dedicated checkout the session will use, never from a working
# copy you edit: every run resets that checkout to origin/main.
set -uo pipefail

cmd="${1:-check}"
machine="${2:-macbook}"
at="${3:-04:40}"
repo="$(cd "$(dirname "$0")/../../.." && pwd)"
label=com.buildwarden.native-tick
plist="$HOME/Library/LaunchAgents/$label.plist"
state="$HOME/Library/Application Support/buildwarden-native"
export PATH="$HOME/.local/bin:$HOME/.local/share/mise/shims:/opt/homebrew/bin:/usr/local/bin:$PATH"

problems=0
ok() { printf '  ok    %s\n' "$*"; }
bad() { printf '  FAIL  %s\n' "$*"; problems=$((problems + 1)); }
warn() { printf '  warn  %s\n' "$*"; }

check() {
	echo "Checking $repo for machine $machine"
	[ "$(uname -s)/$(uname -m)" = Darwin/arm64 ] && ok "macOS arm64 ($(sw_vers -productVersion))" ||
		bad "not macOS arm64: $(uname -s)/$(uname -m)"
	for t in git go make claude; do
		command -v "$t" >/dev/null && ok "$t: $(command -v "$t")" || bad "$t not on PATH"
	done
	if command -v go >/dev/null; then
		want=$(awk '/^go /{print $2}' "$repo/tools/workflow/go.mod")
		(cd "$repo/tools/workflow" && go version >/dev/null 2>&1) &&
			ok "go builds tools/workflow (needs $want)" || bad "go cannot build tools/workflow (needs $want)"
	fi
	command -v entire >/dev/null && ok "entire: sessions will be recorded (FLOW-18)" ||
		warn "entire not installed: sessions will not be recorded (FLOW-18)"
	command -v qemu-system-aarch64 >/dev/null && ok "qemu (macos-qemu)" || warn "no qemu-system-aarch64: macos-qemu work will fail"
	command -v finch >/dev/null && ok "finch (macos-finch)" || warn "no finch: macos-finch work will fail"
	command -v codesign >/dev/null && ok "codesign (macos-vz entitlement)" || bad "no codesign: install the Xcode command line tools"
	url=$(git -C "$repo" remote get-url origin 2>/dev/null)
	case "$url" in
	*buildwarden/buildwarden.git | *buildwarden/buildwarden) ok "origin is $url" ;;
	*) bad "origin is '$url', want buildwarden/buildwarden" ;;
	esac
	[ -z "$(git -C "$repo" status --porcelain --untracked-files=no)" ] && ok "checkout is clean" ||
		bad "checkout has local changes; use a dedicated clone"
	git -C "$repo" fetch -q origin workflow && ok "can fetch origin" || bad "cannot fetch origin"
	git -C "$repo" push --dry-run -q origin "refs/remotes/origin/workflow:refs/heads/workflow" 2>/dev/null &&
		ok "can push the workflow branch" || bad "cannot push the workflow branch (credentials?)"
	free_gb=$(df -Pk "$repo" | awk 'NR==2 {print int($4/1024/1024)}')
	[ "$free_gb" -ge 40 ] && ok "${free_gb}G free" || warn "${free_gb}G free; runs skip below 40G (BW_MIN_FREE_GB)"
	if git -C "$repo" show origin/workflow:hosts.yaml 2>/dev/null | grep -q "name: $machine\$"; then
		ok "hosts.yaml names machine $machine"
	else
		bad "hosts.yaml on origin/workflow has no machine $machine"
	fi
	if [ -f "$plist" ]; then ok "installed: $plist"; else warn "not installed yet"; fi
	pmset -g sched 2>/dev/null | grep -qi wake && ok "a scheduled wake is set" ||
		warn "no scheduled wake: sudo pmset repeat wakeorpoweron MTWRFSU ${wake:-04:35}:00"
	echo "claude must be logged in for headless runs: run 'claude' once in this checkout if unsure."
	[ $problems -eq 0 ] && echo "Ready." || echo "$problems problem(s)."
}

install() {
	h=${at%%:*}
	m=${at##*:}
	h=$((10#$h))
	m=$((10#$m))
	t=$(((h * 60 + m + 1435) % 1440)) # five minutes earlier
	wake=$(printf '%02d:%02d' $((t / 60)) $((t % 60)))
	check || true
	[ $problems -eq 0 ] || { echo "Fix the problems above first."; exit 1; }
	mkdir -p "$(dirname "$plist")" "$state"
	cat >"$plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>$label</string>
  <key>ProgramArguments</key>
  <array>
    <string>/bin/bash</string>
    <string>$repo/tools/workflow/native/native-tick.sh</string>
    <string>$machine</string>
  </array>
  <key>StartCalendarInterval</key>
  <dict><key>Hour</key><integer>$h</integer><key>Minute</key><integer>$m</integer></dict>
  <key>StandardOutPath</key><string>$state/launchd.log</string>
  <key>StandardErrorPath</key><string>$state/launchd.log</string>
  <key>ProcessType</key><string>Background</string>
  <key>LowPriorityIO</key><false/>
</dict>
</plist>
PLIST
	launchctl bootout "gui/$(id -u)/$label" 2>/dev/null || true
	launchctl bootstrap "gui/$(id -u)" "$plist"
	echo "Installed $label: daily at $at for machine $machine (a missed slot runs on wake)."
	echo "Logs: $state/logs/"
	echo "To wake the Mac for it (needs your password):"
	echo "  sudo pmset repeat wakeorpoweron MTWRFSU $wake:00"
	echo "Try one run now: launchctl kickstart gui/$(id -u)/$label"
	echo "  or, ignoring the gate: BW_FORCE=1 $repo/tools/workflow/native/native-tick.sh $machine"
}

case "$cmd" in
check) check; [ $problems -eq 0 ] ;;
install) install ;;
uninstall)
	launchctl bootout "gui/$(id -u)/$label" 2>/dev/null || true
	rm -f "$plist"
	echo "Removed $label. Logs and the gate stamp stay in $state; any pmset repeat stays until 'sudo pmset repeat cancel'."
	;;
*) echo "usage: install-macos.sh check|install|uninstall [machine] [HH:MM]"; exit 2 ;;
esac
