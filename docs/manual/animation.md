[Charcoal](../../README.md) · [Manual](README.md)

# Animation templates, BBS pauses and Animate a folder

## Animation templates

![TheDraw's Clock pattern on ansilove's test art, made into frames and saved as a GIF](../screenshots/animate.gif)

TheDraw 4.63 could turn any finished screen into an animation by drawing it cell by cell in a pattern. CharEdit
does the same (Alt+J, **Animate**): TheDraw's fourteen patterns (Top, Left, Clock, Circles, Checkerboard, Angle,
Gate, Pyramid, Squares, Scramble, Door, Diamond, Wiggle, Wiggle out) with its two toggles **Reverse** and **Flip**,
and Charcoal's own: Fade in / out through the palette, Typewriter, Scroll in, and ten more:

| Template | What it does |
|---|---|
| Matrix rain | in every column a stream of changing glyphs falls (bright head, darker trail; reverse where the page has no colours to spare) and leaves the picture behind; columns start at random times |
| Decrypt | every cell shows random glyphs until it locks to its own character with a flash, sweeping left to right, ragged |
| Spiral | a draw order: clockwise from the outer edge inwards |
| Drop | every character falls from the top row to its place, faster and faster, columns filling from the bottom in a wave |
| Assemble | every character flies in a straight line from a random place to its own, slowing down as it lands |
| Blinds | a draw order: venetian blinds, bands of four rows opening at the same time |
| Shine | a slanted band of light passes once over the finished picture, a logo's glint |
| Shade in | every cell comes up through the page's shade steps (PC: light, medium, dark shade) to its own character |
| Melt in | Doom's screen melt the other way round: the columns slide down into place, each a little behind or ahead of its neighbour |
| Power on | a television switched on: a bright line grows across the middle, then opens up and down, its edges bright |

Each keeps its page's rules: teletext codes stay where they are, Spectrum ink and paper share their brightness, the
Atari 8-bit, PET and ST high resolution use reverse. Random parts come from a seed kept with the page (Shuffle or
S picks another), so saving twice gives the same file. A preview plays the effect at its own speed.

- **Draw order**: saving as `.ans`, `.seq`, `.ata` or `.vt` writes the picture in that order: a clear screen, then
  each cell with a cursor move where it does not follow the one before (ANSI `ESC[row;colH`, PETSCII cursor keys
  and HOME, ATASCII cursor keys, VT52 `ESC Y`), colours only where they change, empty cells left out. Any BBS
  terminal plays it at modem speed; opened again it is exactly the same picture. The order stays with the page
  until changed or cleared; Ctrl+P (and CharView's `m`) plays it. The frame effects (Matrix rain, Decrypt, Drop,
  Assemble, Shine, Shade in, Melt in, Power on) save as a **delta stream**: the clear screen, then each frame's
  changed cells only (the same cursor moves and colours); the modem's speed is the pace, unless BBS pauses are on.
- **Frames**: one every N cells (or every N ms at the modem speed), for the strip, Play, GIF and APNG. Erase makes
  the picture go away instead (TheDraw's Fadeout). Teletext and Spectrum pages make frames only.

## Size, detail and what a caller waits

An animated file is paced by the caller's line: every byte is 10 bits, so 100 KB keeps a caller at 14400 bps
waiting over a minute. Each template has a **Detail** level (D in the requester):

- **Small**, for modems: a draw order puts down runs of up to 8 cells together (about a tenth more than the plain
  file); the frame effects take fewer frames, held longer so they last as long, with little or no glyph churn,
  short trails, short flights and fewer shade steps.
- **Medium** (the default), between the two.
- **Large**, the full look.

Every level saves smaller than before: a gap of up to three cells on a row is crossed by printing its cells again
when that is shorter than a cursor move, a back space steps left, a space keeps the pen's colours where only its
background shows, and each frame of a frame effect writes its cells row by row, column by column or colour by
colour, whichever is shortest.

![Animate with Matrix rain on a 15 KB screen: the cost at each speed and a warning](../screenshots/animate-cost.png)

The requester shows the cost before anything is applied: the lists mark each template light, medium or heavy on
this page at this level (one, two or three bars: up to 1.5x, up to 4x, more than 4x the plain file); under them
the saved file's size and how long it plays at 2400, 9600, 14400, 33600 and 57600 bps and over telnet (the bytes x
10 / bps, plus the board's pauses; over telnet only the pauses count), and a warning when the time at the
caller's speed is past the limit in **Settings** (30 s). The preview plays the saved file as a caller at the
chosen speed sees it, with the board's pauses, and so does Play (Ctrl+P); the speed is the one in the bar
(2400 to 115200 bps, or telnet).

Sizes on a real screen (us-promo.ans from SUPA02, 80x276, 14.5 KB saved plain, 10 s at 14400 bps), in KB:
before these changes, and now at each level:

| Template | Before | Large | Medium | Small | Small at 14400 |
|---|---:|---:|---:|---:|---:|
| Top | 10.4 | 10.0 | 10.0 | 10.0 | 7 s |
| Left | 31.7 | 25.5 | 18.4 | 13.9 | 10 s |
| Clock | 42.5 | 41.2 | 21.2 | 14.8 | 11 s |
| Circles | 34.0 | 33.2 | 18.8 | 13.3 | 9 s |
| Checkerboard | 13.9 | 13.5 | 13.2 | 12.4 | 9 s |
| Angle | 34.7 | 32.7 | 20.3 | 14.5 | 10 s |
| Gate | 35.7 | 33.5 | 18.9 | 13.8 | 10 s |
| Pyramid | 37.8 | 37.3 | 20.3 | 14.4 | 10 s |
| Squares | 32.5 | 28.5 | 17.7 | 13.2 | 9 s |
| Scramble | 46.7 | 46.4 | 23.0 | 15.6 | 11 s |
| Door | 27.1 | 25.9 | 16.5 | 12.4 | 9 s |
| Diamond | 33.3 | 33.1 | 19.3 | 14.2 | 10 s |
| Wiggle | 15.6 | 12.6 | 12.5 | 11.0 | 8 s |
| Wiggle out | 15.6 | 12.6 | 12.3 | 10.9 | 8 s |
| Fade in | 33.6 | 31.6 | 25.4 | 16.5 | 12 s |
| Typewriter | 14.1 | 11.1 | 11.1 | 11.1 | 8 s |
| Matrix rain | 472 | 327 | 158 | 44.0 | 31 s |
| Decrypt | 267 | 202 | 93.0 | 29.7 | 21 s |
| Spiral | 21.5 | 18.1 | 16.5 | 13.0 | 9 s |
| Drop | 230 | 163 | 75.2 | 34.2 | 24 s |
| Assemble | 708 | 494 | 198 | 70.3 | 50 s |
| Blinds | 41.1 | 35.4 | 21.6 | 15.2 | 11 s |
| Shine | 32.3 | 26.7 | 19.6 | 12.7 | 9 s |
| Shade in | 83.0 | 60.6 | 38.7 | 22.4 | 16 s |
| Melt in | 134 | 98.3 | 46.8 | 28.4 | 20 s |
| Power on | 18.2 | 16.2 | 12.9 | 10.6 | 8 s |

## BBS pauses

A modem was the pace: over telnet with no speed limit an animated file arrives at once and the effect flashes past.
Many BBSes read a pause code of their own while they show a screen, so CharEdit can write that board's codes into
the saved `.ans` or `.seq`: a pause every N cells of a draw order, after every frame of a frame effect, or each frame's hold
for the frame strip. **Settings > My BBS** (default *none*) is used for every animated file you save; **Animate**
shows it and can choose another board, or none, for one page (kept with the page's order), and can save one copy
per board in one go beside the file (`name-amiexpress.ans`, `name-mystic.ans` ...). *None* is the safe choice for
art packs, 16colo.rs and several boards: another board's codes show as junk text. The board takes the codes out,
so the terminal gets exactly the plain file's bytes (tested through a model of each board's display code).
Mystic, PCBoard and Synchronet files get the board's own colour codes too (`|nn`, `@Xnn`, Ctrl-A), smaller than
ANSI's and the board's own kind of screen, unless a colour has no code there (blink on Mystic 1.10, italic on an
Amiga page): then all the colours stay ANSI.

| Board | Written first | Pause | Unit, most per code | The board's own character in the art | How known |
|---|---|---|---|---|---|
| Ami-Express (/X) 5 | `~NS\|` (the first byte must be `~`; no More?) | `~<n>w\|` | 1/50 s, 999 | `~` as `~~\|`; codes never cut by its 998-byte reads | source |
| Mystic | `\|PO` (no More?) | `\|DE` | 0.5 s, fixed | `\|` as `\|PI` | source (1.10) + docs |
| PCBoard 15 | `@POFF@` (no More?) | `@DELAY:n@` | 0.1 s, 255 | none exists; codes never cut by its 2047-byte lines | source |
| Synchronet | | Ctrl-A `,` `;` `.` | 0.1 / 0.5 / 2 s | Ctrl-A as Ctrl-A `A`, Ctrl-Z as Ctrl-A `z`; Ctrl-A `Q` after `@`, `\|` and LF | source |
| Tempest | | `~DELAY(n)` | 1/50 s | `~` as `~~` | manual only |
| Image BBS (C64) | | `£Sx` before the next character that shows, `£S0` after it | 0.1 s, 1.5 s (longer: spread over the next characters) | `£` as `££`; never a code cut by its 80-byte lines, a line never starts with `↑` (it ends the file), a RETURN at the end (or the last line is lost) | source (1.2, 2.0) |
| C*Base (C64) | | Ctrl-P and a reverse code that changes nothing | about 1/30 s (two jiffies) | `£` and `@` with a reverse code after them (`£N`, `@:` ... are codes); never cut by its 255-byte lines | source (3.3.8) |
| FAME (whole seconds) | `~0\|` (the first byte must be `~`) | `~Wn\|` | 1 s, 10 | `~` as `~~` | its guide |
| CNet (whole seconds) | Ctrl-Q `@6}` (no word wrap, no More?) | Ctrl-Q `Wn}` | 1 s | Ctrl-Q and Ctrl-Y (◄ ↓) become spaces: nothing escapes them | its MCI help (3.05c) |
| Image BBS 1.2 (C64, whole seconds) | | `£Wx` | 1 s, 15 (the first second is short; Image 2.0 does not wait) | as Image's `£S` | source (1.2) |

The boards marked *(whole seconds)* wait whole seconds only: each frame's hold becomes the nearest whole second
(at least one), a draw order's short pauses are gathered into whole seconds (the total kept), and the play times
CharEdit shows are what the board waits.

Not offered: boards with no fixed delay code (System-X, Sigma Express, DayDream, ENiGMA½, Talisman, Wildcat,
Logic), with a fixed 0.8 s (Renegade `%DE`, Telegard `@8`), Hydra (its timed pause shows a prompt) and Ami-Express
4.x (its manual has no `~w`); on the C64, Color 64 (only `£v`, a speed of unknown units), C-Net 64/128, DMBBS and
Centipede (their codes are not known here). ATASCII and VT52 files get no codes.

A file in a board's colour codes (pipe, Ctrl-A, WWIV, Wildcat!, Celerity, Telegard) saved with a draw order, an
effect, or frames with a board's pauses is written as its animation **in its own codes** (the board's pauses
too), as is every such file in Animate a folder.

## Animate a folder

For a sysop with dozens of screens (menus, logon, bulletins) and no wish to open each: **Animate a folder**
(Alt+Shift+J, or the button in the Open requester) animates every art file of a folder (and its subfolders, if
ticked) for your board in one go, into a new folder (`<folder>-animated`, the names kept). The originals are
never touched unless you tick *Replace the originals*, and then each is kept as `.bak`.

![The Animate a folder requester](../screenshots/animate-folder.png)

- **Effect**: one for every file, or *Random*: a different one for each, from all of them or the ticked ones,
  never the same twice in a row, Reverse and Flip at random. The seed is shown and can be typed: the same seed
  gives the same files again.
- **Pace and board** as in Animate: a pause every N cells, the frame effects' hold, My BBS / None / a board, and
  copies for other boards.
- **Detail** as in Animate (Medium by default), **Effects**: any weight, light only, or light and medium (weighed
  on each screen), and a **budget**: at most N KB a screen and/or plays in under N s at a speed. With a budget each
  version gets an effect that fits: the chosen level, then lower ones, then another effect; a screen nothing fits
  is skipped and the report says what the smallest would have cost. The ticked list shows each effect's weight on a
  made-up typical screen.
- **Versions of each screen**: N animations of every screen, each with its own effect, named the way the board
  picks one at random:

| Board | Versions | How the board picks one |
|---|---|---|
| Ami-Express 5 | `001.LOGON.TXT`, `002.LOGON.TXT` ... | `LOGON.TXT` holds `~3SR_BBS:Node0/Screens/LOGON\|` (the screens folder is asked for) |
| Mystic 1.10 | `logon.ans`, `logon.an1` ... `logon.an8` (9 at most) | by itself, when `logon.an1` is there |
| Synchronet | `logon.ans`, `logon1.ans`, `logon2.ans` ... | a menu name with a wildcard: `bbs.menu("logon*")`, `@MENU:logon*@` (the stock logon.js shows `logon`, `logon1` ... one after another) |
| PCBoard, Tempest, FAME, CNet, none | `LOGON.ANS`, `LOGON-2.ANS` ... | no random screens known: just numbered (FAME's `~SR_` cycles `1-NAME`, `2-NAME` ...: not written yet) |

- A **report** at the end: each file, the effect (and the seed of the effects with random parts), the size before
  and after, each output's size and play time at the budget's speed, and what was skipped and why (not art, a teletext or Spectrum page, already an animation, a
  PETSCII file on a board with codes for `.ans` only ...). It is saved as `animate-report.txt` in the output
  folder, with the command line that repeats the run.

For scripts, the same without a window:

```
charedit --animate FOLDER|FILES... [--effect NAME|random] [--bbs amiexpress|mystic|pcboard|synchronet|tempest|fame|cnet|imagebbs|imagebbsw|cbase|none]
                   [--variants N] [--seed N] [--out DIR] [--subfolders] [--replace]
                   [--detail small|medium|large] [--max-kb N] [--max-seconds N --at BPS|telnet] [--light] ...
```

It prints the report; `charedit --help` lists every option, `--effects` the effects. Exit status 0: done; 1: a
file could not be written or nothing was animated; 2: a bad command line.
