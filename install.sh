#!/bin/sh
#
# Installs git-gui from its GitHub releases.
#
#   curl -fsSL https://raw.githubusercontent.com/o19k/git-gui/main/install.sh | sh
#
# Environment:
#   VERSION   tag to install (default: the latest release)
#   BINDIR    where to put the binary (default: the first writable of
#             /usr/local/bin, $HOME/.local/bin)

set -eu

REPO="o19k/git-gui"
BIN="git-gui"

die() {
	echo "install: $*" >&2
	exit 1
}

info() { echo "install: $*" >&2; }

# --- how to fetch -----------------------------------------------------------

if command -v curl >/dev/null 2>&1; then
	fetch() { curl -fsSL "$1"; }
	fetch_to() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
	fetch() { wget -qO- "$1"; }
	fetch_to() { wget -qO "$2" "$1"; }
else
	die "needs curl or wget"
fi

# --- what to fetch ----------------------------------------------------------

os=$(uname -s)
case "$os" in
Darwin) os=darwin ;;
Linux) os=linux ;;
MINGW* | MSYS* | CYGWIN*)
	die "Windows: download git-gui_windows_amd64.exe from
       https://github.com/$REPO/releases and put it on your PATH"
	;;
*) die "unsupported system: $os" ;;
esac

arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) die "unsupported architecture: $arch" ;;
esac

version="${VERSION:-}"
if [ -z "$version" ]; then
	# sed rather than jq, which may not be installed.
	version=$(fetch "https://api.github.com/repos/$REPO/releases/latest" |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
		head -n 1)
	[ -n "$version" ] || die "cannot determine the latest release — set VERSION=vX.Y.Z"
fi

asset="${BIN}_${os}_${arch}"
base="https://github.com/$REPO/releases/download/$version"

# --- where to put it --------------------------------------------------------

if [ -n "${BINDIR:-}" ]; then
	target="$BINDIR"
elif [ -w /usr/local/bin ] 2>/dev/null; then
	target=/usr/local/bin
else
	# No sudo: a script piped from the network should not decide to run as root.
	target="$HOME/.local/bin"
fi
mkdir -p "$target" || die "cannot create $target"
[ -w "$target" ] || die "$target is not writable — set BINDIR to somewhere it is"

# --- fetch, verify, install -------------------------------------------------

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

info "downloading $BIN $version ($os/$arch)"
fetch_to "$base/$asset" "$tmp/$BIN" || die "no such release asset: $asset ($version)"

if fetch_to "$base/$asset.sha256" "$tmp/sum" 2>/dev/null; then
	# Compare digests: the published sum names the asset, the local file differs.
	want=$(cut -d' ' -f1 <"$tmp/sum")
	if command -v sha256sum >/dev/null 2>&1; then
		got=$(sha256sum "$tmp/$BIN" | cut -d' ' -f1)
	elif command -v shasum >/dev/null 2>&1; then
		got=$(shasum -a 256 "$tmp/$BIN" | cut -d' ' -f1)
	else
		got=""
		info "no sha256 tool — skipping the checksum"
	fi
	if [ -n "$got" ] && [ "$got" != "$want" ]; then
		die "checksum mismatch: got $got, expected $want"
	fi
else
	info "no published checksum for this release — skipping verification"
fi

chmod +x "$tmp/$BIN"

# Cross-device mv is not atomic, so land on the target filesystem first.
mv "$tmp/$BIN" "$target/$BIN.new" || die "cannot write to $target"
mv "$target/$BIN.new" "$target/$BIN"

info "installed $target/$BIN"

case ":$PATH:" in
*":$target:"*) ;;
*) info "note: $target is not on your PATH" ;;
esac

"$target/$BIN" -version
