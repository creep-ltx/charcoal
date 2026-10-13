# The big check

Thousands of real art files, opened, checked, saved and opened again (`bigcheck_test.go`).

1. Get files into `/tmp/bigcheck/files/<group>/` (data only, never run):
   - `fetch-16colors.sh`: 16colo.rs packs, the 15 tagged amiga and two a year 1992-2024 (into `zips/`).
   - `fetch-16colors-tags.sh`: the packs tagged xbin, petscii, ansimation.
   - `fetch-c64-github.sh`: .petmate and .c files found with `gh search code` (needs `pm.list`, see the script);
     .seq from `github.com/sblendorio/petscii-bbs`; ATASCII from breakintochat.com/collections/atascii.
   - Unzip only the art (`unzip -j '*.ans' '*.asc' '*.bin' '*.xb' '*.nfo' '*.diz' '*.txt' ...`).
2. `BIGCHECK_DIR=/tmp/bigcheck/files go test -tags bigcheck -run BigCheck -timeout 25m -v`
3. Read `/tmp/bigcheck/report.txt`; `/tmp/bigcheck/samples/` has a PNG of every 25th file for a contact sheet.

For each file: no crash; the page type against SAUCE where the file says (Amiga / IBM font) and against the kind
(.seq/.c/.petmate C64, .xb PC, the ATASCII collection Atari); not empty; saved in its own format and read back
cell for cell; a PNG. Links, macOS `._` files and web pages saved in place of files are skipped.

2026-10-13: 4119 files, 0 issues, every file with a known type typed right (see the commit for what it found).
