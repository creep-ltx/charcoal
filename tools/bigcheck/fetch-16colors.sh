#!/usr/bin/env bash
# Downloads art packs from 16colo.rs for the big check. Data only; nothing here is run.
cd /tmp/bigcheck/zips
get_pack() { # dir pack
  local link
  link=$(curl -sSL --max-time 20 "https://16colo.rs/pack/$2/" | grep -o -E 'href="/archive/[0-9]+/[^"]*\.zip"' | head -1 | sed 's/href="//; s/"$//')
  [ -n "$link" ] && curl -sSL --max-time 60 -o "$1/$2.zip" "https://16colo.rs$link"
  sleep 1
}
for p in $(curl -sSL --max-time 20 'https://16colo.rs/tags/content/amiga' | grep -o 'href="/pack/[^"/]*' | sed 's|href="/pack/||' | sort -u); do get_pack amiga "$p"; done
for y in $(seq 1992 2 2024); do
  for p in $(curl -sSL --max-time 20 "https://16colo.rs/year/$y/" | grep -o 'href="/pack/[^"/]*' | sed 's|href="/pack/||' | sort -u | shuf -n 2 --random-source=<(yes $y)); do get_pack pc "$p"; done
done
