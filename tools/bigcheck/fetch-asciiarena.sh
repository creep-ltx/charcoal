#!/usr/bin/env bash
# A sample of asciiarena.se releases (Amiga ASCII art) for the big check. Data only.
cd /tmp/bigcheck/web
mkdir -p /tmp/bigcheck/files/asciiarena
grep -E '/release/.*\.(txt|asc|ans)$' urls.txt | shuf -n 600 --random-source=<(yes 42) | while read -r u; do
  f="${u##*/release/}"
  stem="${f%.*}"
  enc_stem=$(python3 -I -c 'import sys,urllib.parse;print(urllib.parse.quote(sys.argv[1], safe=""))' "$stem")
  enc_f=$(python3 -I -c 'import sys,urllib.parse;print(urllib.parse.quote(sys.argv[1], safe=""))' "$f")
  out="/tmp/bigcheck/files/asciiarena/$(echo "$f" | tr '/' '_')"
  [ -s "$out" ] && continue
  curl -sSL --max-time 20 -o "$out" "https://asciiarena.se/collections/$enc_stem/$enc_f"
  sleep 0.4
done
ls /tmp/bigcheck/files/asciiarena | wc -l
