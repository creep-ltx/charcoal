#!/usr/bin/env bash
# Puts the CPC 6128's ROMs where MAME finds them for the emulator check:
# ~/.cache/charcoal-emu/roms/cpc6128/{cpc6128.rom,cpcados.rom}, from Caprice32's
# rom/ folder (Amstrad allows passing its ROMs on; see Caprice32's rom/COPYRIGHT.TXT).
# MAME's checksums: cpc6128.rom CRC 9e827fe1, cpcados.rom CRC 1fe22ecd.
set -euo pipefail
cache=${CHARCOAL_EMU_DIR:-$HOME/.cache/charcoal-emu}
dir=$cache/roms/cpc6128
mkdir -p "$dir"
base=https://raw.githubusercontent.com/ColinPitrat/caprice32/master/rom
curl -sfL -o "$dir/cpc6128.rom" "$base/cpc6128.rom"
curl -sfL -o "$dir/cpcados.rom" "$base/amsdos.rom"
sha1sum "$dir"/*.rom
echo "want: 5977adbad3f7c1e0e082cd02fe76a700d9860c30 cpc6128.rom, 39102c8e9cb55fcc0b9b62098780ed4a3cb6a4bb cpcados.rom"
