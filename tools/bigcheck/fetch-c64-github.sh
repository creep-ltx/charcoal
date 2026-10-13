#!/usr/bin/env bash
# Real .petmate and .c PETSCII files found on GitHub. Data only.
cd /tmp/bigcheck/files/c64
while IFS=$'\t' read -r repo file; do
  n=$(echo "$repo/$file" | md5sum | cut -c1-6)
  curl -sSL --max-time 30 -o "petmate/${n}_$(basename "$file")" "https://raw.githubusercontent.com/$repo/HEAD/$file"
  sleep 0.3
done < pm.list
gh search code "border,bg,chars,colors" --extension c --limit 40 --json repository,path 2>/dev/null | python3 -I -c '
import json,sys
for r in json.load(sys.stdin): print(r["repository"]["nameWithOwner"]+"\t"+r["path"])' > c.list
while IFS=$'\t' read -r repo file; do
  n=$(echo "$repo/$file" | md5sum | cut -c1-6)
  curl -sSL --max-time 30 -o "c/${n}_$(basename "$file")" "https://raw.githubusercontent.com/$repo/HEAD/$file"
  sleep 0.3
done < c.list
echo "petmate $(ls petmate | wc -l), c $(ls c | wc -l)"; du -sh .
