#!/usr/bin/env bash
# Builds a800shot (atari800's core without a window, see a800shot.c) in
# ~/.cache/charcoal-emu: atari800 7.2.1's source from GitHub, configured as
# libatari800, and a800shot linked against it. Needs git, autoconf, automake, a C compiler.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
cache=${CHARCOAL_EMU_DIR:-$HOME/.cache/charcoal-emu}
src=$cache/src/atari800
mkdir -p "$cache/src" "$cache/bin"
if [ ! -d "$src" ]; then
	git clone --depth 1 --branch ATARI800_7_2_1 https://github.com/atari800/atari800.git "$src"
fi
cd "$src"
if [ ! -f src/libatari800.a ]; then
	[ -f configure ] || ./autogen.sh
	./configure --target=libatari800 CFLAGS="-O2 -Wno-error"
	# configure adds -Werror, and newer compilers warn about its code
	make -j4 CFLAGS="-O2 -DNETSIO -Wno-error"
fi
# the libraries configure found (lame, zlib, png...) as its Makefile links them
libs=$(sed -n 's/^LIBS = //p' src/Makefile)
cc -O2 -I src -I src/libatari800 -o "$cache/bin/a800shot" "$here/a800shot.c" src/libatari800.a $libs
echo "built $cache/bin/a800shot"
