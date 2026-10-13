#!/usr/bin/env bash
# Copies (never moves) local and NAS art into the big check. Data only.
set -u
dst=/tmp/bigcheck/files
mkdir -p "$dst/bbsstyle" "$dst/nas-petscii" /tmp/bigcheck/nas-png
src=~/Amiga/BBS/"BBS Style"
find "$src" -type f \( -iname '*.ans' -o -iname '*.asc' -o -iname '*.txt' -o -iname '*.nfo' -o -iname '*.anim' -o -iname '*.diz' \) -print0 |
  while IFS= read -r -d '' f; do cp -n "$f" "$dst/bbsstyle/$(echo "$f" | md5sum | cut -c1-6)_$(basename "$f")"; done
nas=/mnt/nas/home/Unsorted/Junk/Latex
find "$nas/Petscii" "$nas/Petscii Unsorted" -type f \( -iname '*.c' -o -iname '*.seq' -o -iname '*.prg' \) -print0 |
  while IFS= read -r -d '' f; do
    n="$(echo "$f" | md5sum | cut -c1-6)_$(basename "$f")"
    cp -n "$f" "$dst/nas-petscii/$n"
    png="${f%.*}.png"   # the PETSCII editor's own picture of the same art
    [ -f "$png" ] && cp -n "$png" "/tmp/bigcheck/nas-png/${n%.*}.png"
  done
echo "bbsstyle $(ls "$dst/bbsstyle" | wc -l), nas-petscii $(ls "$dst/nas-petscii" | wc -l), editor pictures $(ls /tmp/bigcheck/nas-png | wc -l)"
