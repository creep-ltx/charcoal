[Charcoal](../../README.md) · [Manual](README.md)

# Licences

Charcoal's own code is under the MIT licence (`LICENSE`), copyright Tobias Karlsson. These parts keep theirs:

| Part | From | Licence |
|---|---|---|
| Amiga fonts (Topaz 1/1+/2/2+, MicroKnight/+, P0T-NOoDLE, mO'sOul) | TrueSchool's *Multi Platform Amiga Fonts* raw conversions, dMG/t!s^dS!, github.com/rewtnull/amigafonts (256 glyphs × 16 rows, each row of the 8×8 original doubled) | GPL with the font exception, `fonts/amigafonts-NOTICE.txt` |
| The Go font (the interface's bundled font) | golang.org/x/image/font/gofont, by Bigelow & Holmes for the Go project | BSD 3-clause, in the module |
| IBM VGA 8×16 | rendered from Px437_IBM_VGA_8x16, VileR, *The Ultimate Oldschool PC Font Pack* | CC BY-SA 4.0, `fonts/Px437-LICENSE.TXT` |
| IBM EGA 8×14, IBM VGA 8×8, IBM VGA 8×16 code pages 850 and 866 (`fonts/IBM_*.f08/.f14/.f16`) | drawn from Bm437_IBM_EGA_8x14, Bm437_IBM_VGA_9x8 and BmPlus_IBM_VGA_8x16, VileR, *The Ultimate Oldschool PC Font Pack* v2.2 (int10h.org), by `tools/pcfonts.py` | CC BY-SA 4.0, `fonts/Px437-LICENSE.TXT` (the pack's licence) |
| Atari 8-bit fonts | AltirraOS, from the Altirra emulator | all-permissive, `fonts/AltirraOS-LICENSE.txt` |
| Atari GTIA colours, the default (`gtia.go` `altirraTable`) | the model of Altirra's palette generator, Avery Lee (Altirra 4.40, `palettegenerator.cpp` and its default PAL/NTSC presets, virtualdub.org/altirra.html): luma ramp, hue angles, saturation, PAL phase table, colour matrices; written anew, no code taken | facts (numbers); Altirra itself is GPL-2+ |
| Atari GTIA colours, atari800's (`gtia.go` `gtiaTable`) | the model and measured numbers of atari800's palette generator (`colours_ntsc.c`, `colours_pal.c`, github.com/atari800/atari800): luma steps, colour delays, PAL delay table; written anew from its description, no code taken | facts (numbers); atari800 itself is GPL-2+ |
| ZX Spectrum font | the 48K ROM's character set | © Amstrad, who allow passing it on, `fonts/zx-spectrum-NOTICE.txt` |
| Amstrad CPC font | the CPC firmware ROM's character set (the same in the 464, 664 and 6128) | © Amstrad; Amstrad has kindly given its permission for its copyrighted material to be redistributed but Amstrad retains its copyright, `fonts/amstrad-cpc-NOTICE.txt` |
| MSX fonts (`fonts/msx-cbios*.font`) | C-BIOS 0.29a's character sets (international by Eric Boon, Japanese, Brazilian), cbios.sourceforge.net | BSD 2-clause, `fonts/msx-cbios-NOTICE.txt` |
| Sharp MZ-700 font (`fonts/mz700-rjm.font`) | Russell Marks's hand-drawn MZ-700-like set from mz700em (1996), kept in github.com/mkoeppe/mz800em | public domain, `fonts/mz700-NOTICE.txt` |
| Sharp MZ-700 second set (`fonts/mz700-set2.font`) | CharEdit's own look-alike, drawn for it in `tools/mz700-set2.txt` (2026) | CC0 1.0, `fonts/mz700-set2-NOTICE.txt` |
| Teletext font | Bedstead, Ben Harris | CC0, `fonts/bedstead-NOTICE.txt` |
| Minitel font (`minitel_font.go`) | Minitel-Canvas's Minitel font, Frédéric Bisson (github.com/Zigazou/Minitel-Canvas), drawn into 8x10 cells by `tools/minitel-font.py` | CC0, `fonts/minitel-NOTICE.txt` |
| Unicode page fonts (`fonts/unifont-art.hex.gz`, `fonts/unifont-wide.hex.gz`) | GNU Unifont 17.0.05, Roman Czyborra, Paul Hardy et al. (unifoundry.com): the blocks text art uses, and every wide character (CJK, kana, Hangul, emoji), cut by `tools/unifont.sh` | SIL Open Font License 1.1 (Unifont is dual-licensed OFL / GPL-2+ with the font exception; used under the OFL), `fonts/unifont-NOTICE.txt` |
| durdraw's colour numbers and file format (`uniformats.go`) | durdraw, Sam Foster (github.com/cmang/durdraw), read from its source; no code copied | BSD 3-clause |
| PETSCII flip and turn tables (`cbmturn.go`) | Marq's PETSCII editor, Marq / Fit^L!T, tables by Dr. TerrorZ | WTFPL, `third_party/marq-petscii-LICENSE.txt` |
| Petmate's file formats | Petmate, C. T. Lin | MIT, `third_party/petmate-LICENSE.txt` |
| SDL2 (linked into the Windows and Mac programs) | libsdl.org, Sam Lantinga | zlib, `third_party/SDL2-LICENSE.txt` |
| go-sdl2, the Go binding to SDL2 | github.com/veandco/go-sdl2 | BSD 3-clause, `third_party/go-sdl2-LICENSE.txt` |
| BMP and WebP readers (reference pictures) | golang.org/x/image, The Go Authors | BSD 3-clause, `third_party/x-image-LICENSE.txt` |
| Websocket (drawing sessions) | github.com/coder/websocket v1.8.15, Coder (formerly nhooyr.io/websocket) | ISC, `third_party/coder-websocket-LICENSE.txt` |
| zstd decoder (IcyDraw files) | github.com/klauspost/compress v1.19.0, Klaus Post, The Go Authors (xxhash: Caleb Spare) | BSD 3-clause (and Apache 2.0 parts, MIT xxhash), `third_party/klauspost-compress-LICENSE.txt` |
| Playscii's palettes (`psciPalettes`) and glyph tables | Playscii, JP LeBreton (the colours and glyph order of its palettes/ and charsets/) | MIT, `third_party/playscii-LICENSE.txt` |
| Marq's ACME copy loop (`marqAsmCode`) | Marq's PETSCII editor | WTFPL, `third_party/marq-petscii-LICENSE.txt` |
| The Plus/4 program's head (`tedPrgHead`, `ted_formats.go`) | Marq's PETSCII editor's `template-plus4.prg` | WTFPL, `third_party/marq-petscii-LICENSE.txt` |
| The Plus/4 palettes (`ted.go`) | VICE: its TED colour model (`src/plus4/ted-color.c`, `src/video/video-color.c`) worked out again in Go, and the colour values of its `yape-pal.vpl` (taken from a YAPE screenshot) and `colodore_ted.vpl` (Pepto's colodore.com); colour values only, no VICE code | VICE is GPL-2; colours are measurements |
| `testdata/marq/plus4-stripes.c` | Marq's PETSCII editor's Plus/4 example | WTFPL, `testdata/marq/README` |
| `testdata/petmate9/c16_colorbars.petmate` | Petmate9's C16 test workspace (github.com/wbochar/petmate9) | MIT, `testdata/petmate9/README` |
| Test art in `testdata/alove` | ansilove's tests | BSD 2-clause, `testdata/alove/LICENSE-ansilove.txt` |
| Test files in `testdata/moebius` | written and read back by Moebius's own save and load code (github.com/blocktronics/moebius, `app/libtextmode`), run from a separate checkout by `tools/moebius/gen.js` and `check.js`; no Moebius code is copied | Moebius is Apache-2.0; the generated files are Charcoal's (MIT) |
| `testdata/icydraw` | `testdata/alove`'s art as IcyDraw 0.6 saves it | as above |
| `testdata/playscii/hello1.psci` | Playscii's example art | MIT, `testdata/playscii/LICENSE-playscii.txt` |
| `testdata/charpad` | c64lib's CharPad test projects (gradle-retro-assembler-plugin) | MIT, `testdata/charpad/SOURCE.txt` |
| `testdata/durdraw` | durdraw's example art (cmang, indyz) and durdraw's own ANSI exports of it | BSD 3-clause, `testdata/durdraw/NOTICE.txt` |
| The IGS interpreter's drawing (`igs.go`, `igs_vdi.go`: VDI lines, arcs, fills, patterns, text effects, blits) | ported from IcyDraw's engine (github.com/mkrueger/icy_tools, Mike Krüger); every table and routine from its source, nothing from EmuTOS's (GPL) code | MIT or Apache-2.0 (used under MIT), `third_party/icy_tools-LICENSE-MIT.txt` |
| `testdata/igs/icydraw` | IcyDraw's IGS test scripts and its renders of them | MIT or Apache-2.0, `testdata/igs/icydraw/README` |

Not included, fetched on first use and checked by SHA-1: the Commodore character ROMs (from the VICE
project; for the Plus/4 its KERNAL ROM, which holds the characters) and the EmuTOS font sources (GPL; 8x8, 8x16 and, for IGS's small text, 6x6), for PETSCII and Atari ST
pages; Playscii's charset pictures (Apple II, Agat, BBC Master, Intellivision, Pac-Man, Sharp MZ, ZX81) and
palettes, from Playscii's source at a fixed version (Playscii is MIT, but the glyphs are the machines' own); the
Amiga fonts SkyPix files name (Topaz, the Workbench fonts and the 8x8 IBM set SkyPix terminals draw text in),
as IcyDraw 0.6's source keeps them (Amiga's own fonts). TheDraw fonts: none
come with the program (they are their makers' art, mostly without a stated licence); *Download...* in the
Logo tool's font list fetches Roy/SAC's free collection (`TDFONTSALL.ZIP`, roysac.com) when asked. Not included and not fetched: an Atari OS ROM (Atari's; used only when it is already on the computer).
