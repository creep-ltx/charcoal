#!/usr/bin/env bash
# Builds fonts/unifont-art.hex.gz: the blocks of GNU Unifont that text art uses
# (Latin, Greek, Cyrillic, punctuation, arrows, maths, technical, box drawing,
# blocks, shapes, symbols, dingbats, Braille, Symbols for Legacy Computing and its
# supplement). Unifont is dual-licensed OFL-1.1 / GPL-2+ with the font embedding
# exception (fonts/unifont-NOTICE.txt); Charcoal uses it under the OFL.
#   tools/unifont.sh [version]      (default 17.0.05; run from the repository)
set -euo pipefail
ver=${1:-17.0.05}
base=https://unifoundry.com/pub/unifont/unifont-$ver/font-builds
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
curl -sSfL -o "$tmp/bmp.hex.gz" "$base/unifont-$ver.hex.gz"
curl -sSfL -o "$tmp/upper.hex.gz" "$base/unifont_upper-$ver.hex.gz"
{ gzip -dc "$tmp/bmp.hex.gz"; gzip -dc "$tmp/upper.hex.gz"; } | awk -F: '
function inr(c, a, b) { return c >= a && c <= b }
{
  c = strtonum("0x" $1)
  if (inr(c, 0x20, 0x7E) || inr(c, 0xA0, 0x24F) || inr(c, 0x370, 0x4FF) ||
      inr(c, 0x2000, 0x2BFF) || inr(c, 0x1FB00, 0x1FBFF) || inr(c, 0x1CC00, 0x1CFFF))
    print
}' | gzip -9n > fonts/unifont-art.hex.gz
echo "fonts/unifont-art.hex.gz: $(gzip -dc fonts/unifont-art.hex.gz | wc -l) glyphs, $(stat -c %s fonts/unifont-art.hex.gz) bytes"
