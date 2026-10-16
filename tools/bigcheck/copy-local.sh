#!/usr/bin/env bash
# Copies (never moves) local and NAS art into the big check. Data only.
# Usage: copy-local.sh [BBS_STYLE_DIR [PETSCII_DIR]]
#   BBS_STYLE_DIR (or $BBSSTYLE_DIR): a folder of real BBS art (.ans .asc .txt .nfo .anim .diz).
#   PETSCII_DIR (or $PETSCII_DIR): a folder holding "Petscii" and "Petscii Unsorted" (.c .seq .prg,
#   with the PETSCII editor's own .png of each). Either may be left out; it is then skipped.
set -u
src=${1:-${BBSSTYLE_DIR:-}}
nas=${2:-${PETSCII_DIR:-}}
dst=/tmp/bigcheck/files
mkdir -p "$dst/bbsstyle" "$dst/nas-petscii" /tmp/bigcheck/nas-png
[ -n "$src" ] && find "$src" -type f \( -iname '*.ans' -o -iname '*.asc' -o -iname '*.txt' -o -iname '*.nfo' -o -iname '*.anim' -o -iname '*.diz' \) -print0 |
  while IFS= read -r -d '' f; do cp -n "$f" "$dst/bbsstyle/$(echo "$f" | md5sum | cut -c1-6)_$(basename "$f")"; done
[ -n "$nas" ] && find "$nas/Petscii" "$nas/Petscii Unsorted" -type f \( -iname '*.c' -o -iname '*.seq' -o -iname '*.prg' \) -print0 |
  while IFS= read -r -d '' f; do
    n="$(echo "$f" | md5sum | cut -c1-6)_$(basename "$f")"
    cp -n "$f" "$dst/nas-petscii/$n"
    png="${f%.*}.png"   # the PETSCII editor's own picture of the same art
    [ -f "$png" ] && cp -n "$png" "/tmp/bigcheck/nas-png/${n%.*}.png"
  done
echo "bbsstyle $(ls "$dst/bbsstyle" | wc -l), nas-petscii $(ls "$dst/nas-petscii" | wc -l), editor pictures $(ls /tmp/bigcheck/nas-png | wc -l)"
