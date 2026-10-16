#!/usr/bin/env bash
# Puts C-BIOS 0.29a (cbios.sourceforge.net, a free MSX BIOS, BSD licence) where
# MAME finds it for the emulator check, under the names of two MAME machines:
#   ~/.cache/charcoal-emu/roms/hb10p/10pbios.ic12  (Sony HB-10P, MSX1): cbios_main_msx1_eu.rom
#   ~/.cache/charcoal-emu/roms/phc23/23bios.rom    (Panasonic/Sanyo PHC-23, MSX2): cbios_main_msx2_eu.rom
#   ~/.cache/charcoal-emu/roms/phc23/23ext.rom     (its sub-ROM): cbios_sub.rom
# MAME says "WRONG CHECKSUMS" and runs them. Microsoft's MSX BIOS may not be passed on.
set -euo pipefail
cache=${CHARCOAL_EMU_DIR:-$HOME/.cache/charcoal-emu}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -sfL -o "$tmp/cbios.zip" "https://sourceforge.net/projects/cbios/files/cbios/0.29/cbios-0.29a.zip/download"
(cd "$tmp" && unzip -q cbios.zip)
r=$tmp/cbios-0.29a/roms
mkdir -p "$cache/roms/hb10p" "$cache/roms/phc23"
cp "$r/cbios_main_msx1_eu.rom" "$cache/roms/hb10p/10pbios.ic12"
cp "$r/cbios_main_msx2_eu.rom" "$cache/roms/phc23/23bios.rom"
cp "$r/cbios_sub.rom" "$cache/roms/phc23/23ext.rom"
sha1sum "$cache/roms/hb10p/10pbios.ic12" "$cache/roms/phc23/"*.rom
