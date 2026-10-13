#!/usr/bin/env bash
cd /tmp/bigcheck/zips
get_pack() {
  [ -f "$1/$2.zip" ] && return
  local link
  link=$(curl -sSL --max-time 20 "https://16colo.rs/pack/$2/" | grep -o -E 'href="/archive/[0-9]+/[^"]*\.zip"' | head -1 | sed 's/href="//; s/"$//')
  [ -n "$link" ] && curl -sSL --max-time 90 -o "$1/$2.zip" "https://16colo.rs$link"
  sleep 1
}
for tag in xbin petscii ansimation; do
  mkdir -p "$tag"
  for p in $(curl -sSL --max-time 20 "https://16colo.rs/tags/content/$tag" | grep -o 'href="/pack/[^"/]*' | sed 's|href="/pack/||' | sort -u | head -18); do get_pack "$tag" "$p"; done
done
