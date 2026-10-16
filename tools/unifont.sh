#!/usr/bin/env bash
# Builds Charcoal's two Unifont files from GNU Unifont:
#   fonts/unifont-art.hex.gz   the blocks text art uses (Latin, Greek, Cyrillic,
#                              punctuation, arrows, maths, technical, box drawing,
#                              blocks, shapes, symbols, dingbats, Braille, Symbols
#                              for Legacy Computing and its supplement)
#   fonts/unifont-wide.hex.gz  every character a terminal shows two columns wide
#                              (East Asian Width W or F: CJK, kana, Hangul,
#                              fullwidth forms, emoji) that Unifont draws
# Unifont is dual-licensed OFL-1.1 / GPL-2+ with the font embedding exception
# (fonts/unifont-NOTICE.txt); Charcoal uses it under the OFL.
#   tools/unifont.sh [version] [unicode]   (default 17.0.05 and 17.0.0; run from the repository)
set -euo pipefail
ver=${1:-17.0.05}
uver=${2:-17.0.0}
base=https://unifoundry.com/pub/unifont/unifont-$ver/font-builds
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -sSfL -o "$tmp/bmp.hex.gz" "$base/unifont-$ver.hex.gz"
curl -sSfL -o "$tmp/upper.hex.gz" "$base/unifont_upper-$ver.hex.gz"
curl -sSfL -o "$tmp/eaw.txt" "https://www.unicode.org/Public/$uver/ucd/EastAsianWidth.txt"
{ gzip -dc "$tmp/bmp.hex.gz"; gzip -dc "$tmp/upper.hex.gz"; } > "$tmp/all.hex"
awk -F: '
function inr(c, a, b) { return c >= a && c <= b }
{
  c = strtonum("0x" $1)
  if (inr(c, 0x20, 0x7E) || inr(c, 0xA0, 0x24F) || inr(c, 0x370, 0x4FF) ||
      inr(c, 0x2000, 0x2BFF) || inr(c, 0x1FB00, 0x1FBFF) || inr(c, 0x1CC00, 0x1CFFF))
    print
}' "$tmp/all.hex" | gzip -9n > fonts/unifont-art.hex.gz
sed -e 's/#.*//' "$tmp/eaw.txt" | awk -F';' '
NF >= 2 {
  gsub(/[ \t]/, "", $1); gsub(/[ \t]/, "", $2)
  if ($2 != "W" && $2 != "F") next
  n = split($1, r, /\.\./)
  printf "%d %d\n", strtonum("0x" r[1]), (n > 1) ? strtonum("0x" r[2]) : strtonum("0x" r[1])
}' > "$tmp/wide.txt"
awk -F: 'NR == FNR { lo[NR] = $1 + 0; split($0, p, " "); lo[NR] = p[1] + 0; hi[NR] = p[2] + 0; n = NR; next }
{
  c = strtonum("0x" $1)
  for (i = 1; i <= n; i++) if (c >= lo[i] && c <= hi[i]) { print; break }
}' "$tmp/wide.txt" "$tmp/all.hex" | sort -u | sort -t: -k1,1 | gzip -9n > fonts/unifont-wide.hex.gz
for f in art wide; do
  echo "fonts/unifont-$f.hex.gz: $(gzip -dc fonts/unifont-$f.hex.gz | wc -l) glyphs, $(stat -c %s fonts/unifont-$f.hex.gz) bytes"
done
