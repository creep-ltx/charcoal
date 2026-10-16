# The emulator check

CharEdit's machine files run in real emulators, and each emulator's screen is compared with CharEdit's PNG export
of the same page (`emu_test.go`, off unless `CHARCOAL_EMU=1`):

| Machine | Files | Emulator | How it runs without a window |
|---|---|---|---|
| C64 | `.prg` (Petmate's form; CharEdit's with an own set, multicolour, ECM; upper/lower case), `.d64` | VICE 3.10 `x64sc` | `-console -warp -limitcycles N -exitscreenshot shot.png -autostart` (`.prg` injected: `-autostartprgmode 1`; a `.d64` loads through the 1541) |
| Atari 8-bit | `.xex`, boot `.atr`; 38/40 columns, ROM/own font, short pages; 800XL and 800 | atari800 7.2.1's core | `a800shot` (this folder): libatari800 runs N frames and writes the screen's colour values and its palette. AltirraOS is built into atari800 (`-xl-rev altirra`, `-800-rev altirra`): no ROM files needed |
| ZX Spectrum | `.tap` (loader + SCREEN$), ROM and own font | Fuse 1.9.1 (GTK) | GTK's Broadway backend (`broadwayd`, a display server with no screen, from gtk3) and Fuse's movie (`--movie-start x.fmf --movie-compr None`); the test reads the FMF itself |
| Amstrad CPC | `.dsk` with the `.bin`, modes 0/1/2 | MAME 0.289 `cpc6128` | `-video none -seconds_to_run N -autoboot_command 'run"art\n'`; the snapshot MAME takes when it stops |

What must match: every pixel of the screen and of a 4-pixel ring of border around it shows the same colour, as a
one-to-one pairing of CharEdit's colours with the emulator's (the emulators' palettes differ from CharEdit's, so
RGB is not compared; each test logs its pairs). The screen is found anywhere in the emulator's picture.

## Set up (once)

- Emulators: `x64sc` (vice), `fuse` (fuse-emulator) and `broadwayd` (gtk3), `mame`. A missing one skips its test.
- `./build-a800shot.sh` clones atari800 7.2.1 into `~/.cache/charcoal-emu/src/atari800`, builds it as libatari800
  and links `~/.cache/charcoal-emu/bin/a800shot` (needs git, autotools, a C compiler).
- `./get-cpc-roms.sh` puts the CPC 6128 ROMs (Caprice32's copies; Amstrad allows passing them on) in
  `~/.cache/charcoal-emu/roms/cpc6128/` under MAME's names.
- The C64 ROMs come from VICE's installation (as for CharEdit itself).

## Run

    CHARCOAL_EMU=1 go test -count=1 -run TestEmu -v .

About 80 seconds (Fuse runs 15 seconds a tape). Pictures go to `/tmp/charcoal-emu/` (or `CHARCOAL_EMU_OUT`):
`<case>-emu.png` (the emulator's screen) and `<case>-charedit.png` (CharEdit's export). The `*-text` cases are
readable pages, for a person. `CHARCOAL_EMU_DIR` moves the cache.

Every emulator runs under `timeout` (KILL 5 seconds after an unanswered TERM: Fuse ignores TERM) and, with
systemd, in a scope of at most 4 GB, with `env -i`: no `DISPLAY`/`WAYLAND_DISPLAY`, a temporary `HOME` and
`XDG_*` (none of them reads or writes the user's settings), SDL's dummy drivers. Never run them on the user's
screen. Call MAME as `/usr/lib/mame/mame`: Arch's `/usr/bin/mame` is a script that first writes `~/.mame/`.

## Traps

- VICE's default autostart of a `.prg` goes through a virtual drive and is still "SEARCHING" after 4 million
  cycles; inject it (`-autostartprgmode 1`).
- Broadway's socket is an abstract one (no file in `XDG_RUNTIME_DIR`): wait for broadwayd's "Listening" line.
  `systemd-run --user` itself needs the session's `XDG_RUNTIME_DIR`, so the emulator's own environment is given
  with `env -i` inside the scope.
- Fuse's movie keeps the bitmap and attribute byte of every 8-pixel chunk but not the FLASH phase; bright black
  is black (the test folds colour 8 into 0).
- MAME's CPC picture is 768x272: one emulator pixel per CharEdit pixel across, one per two lines down; the screen
  is at 64,37. VICE's is 384x272 with the screen at 32,35; atari800's 384x240 at 32,24 (48,24 for 38 columns);
  Fuse's 320x240 at 32,24.
