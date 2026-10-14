#!/usr/bin/env bash
# Makes preview/templates.mp4 (and preview/sheet.png): the animation templates
# drawn on real art by CharEdit's own renderer (preview_test.go), put together
# by ffmpeg at 30 pictures a second. Art from $ART (default /tmp/Art).
set -euo pipefail
cd "$(dirname "$0")/.."
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p preview
CHARCOAL_PREVIEW="$work" CHARCOAL_PREVIEW_ART="${ART:-/tmp/Art}" CHARCOAL_REAL_HOME="$HOME" \
	CHARCOAL_PREVIEW_BONUS="${BONUS:-1}" go test -count=1 -run TestTemplatePreview .
ffmpeg -hide_banner -loglevel error -y -f concat -safe 0 -i "$work/list.txt" \
	-vf "fps=30,format=yuv420p" -c:v libx264 -preset slow -crf 20 -movflags +faststart preview/templates.mp4
cp "$work/sheet.png" preview/sheet.png
ls -l preview
