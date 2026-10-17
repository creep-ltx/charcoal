#!/usr/bin/env bash
# Stages one release folder: package.sh linux|windows|macos BUILD_DIR OUT_DIR VERSION
#   linux:   BUILD_DIR/charedit
#   windows: BUILD_DIR/charedit.exe (charview.exe is made as a copy: the name picks the viewer)
#   macos:   BUILD_DIR/charedit (a universal binary) -> Charcoal.app
set -euo pipefail
os=$1 build=$2 out=$3 version=${4:-dev}
root=$(cd "$(dirname "$0")/../.." && pwd)
rm -rf "$out"
mkdir -p "$out/licences"
cp "$root/README.md" "$root/LICENSE" "$out/"
mkdir -p "$out/docs"
cp -R "$root/docs/manual" "$root/docs/screenshots" "$out/docs/"
cp "$root"/fonts/*NOTICE* "$root"/fonts/*LICENSE* "$root"/third_party/*LICENSE* "$out/licences/"
case $os in
linux)
	cp "$build/charedit" "$out/"
	chmod +x "$out/charedit"
	ln -s charedit "$out/charview"
	;;
windows)
	cp "$build/charedit.exe" "$out/charedit.exe"
	cp "$build/charedit.exe" "$out/charview.exe"
	;;
macos)
	app="$out/Charcoal.app"
	mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
	cp "$build/charedit" "$app/Contents/MacOS/charedit"
	chmod +x "$app/Contents/MacOS/charedit"
	case $version in v[0-9]*) plist=${version#v} ;; *) plist=0.0 ;; esac
	sed "s/VERSION/$plist/g" "$root/tools/release/Info.plist" > "$app/Contents/Info.plist"
	printf 'APPL????' > "$app/Contents/PkgInfo"
	cp -R "$out/licences" "$app/Contents/Resources/"
	;;
*)
	echo "unknown system $os" >&2
	exit 2
	;;
esac
