[Charcoal](../../README.md) · [Manual](README.md)

# For compos: party check and work stages

**File > Check for a party...** checks the page against a compo's published rules (read from each party's own
page in October 2026; the link is shown) and lists only what breaks them: the page's size, iCE colours and
blinking, an own font or palette, frames or a draw order (an animation), C64 multicolour/ECM, the wrong page type
or machine, teletext rows a broadcast overwrites. It changes nothing. Rules it knows: Revision (ASCII / ANSI /
PETSCII, and oldskool graphics), Evoke (one-screen ANSI / ASCII), Lovebyte (text graphics of at most 256
characters: on machine pages the box around what is drawn counts), Zoo (PETSCII), Deadline (textmode), X
(PETSCII), Logiker's AMSCII and ATASCII, the Teletext Advent Calendar. It also says what to hand in, how many work
stages, and the rules it cannot check (no AI, no conversions).

**Save for the party** (in the same requester) writes the files that compo wants into a folder beside the page's
file, `name-<compo>/`: e.g. the `.ans` and a PNG, the C64 `.prg` and a PNG, the CPC `.bin` (8-character name),
`.dsk`, PNG and `.psci`, and the work stages in `workstages/` when the compo asks for them.

**File > Export work stages...** writes N pictures of the work (`name-stage01.png` ... at 1x or 2x, and in the
page's own format if ticked), spread evenly from the start of the work to now; the last one is the page as it is.
CharEdit keeps a thin history of every page for this (at most 64 pictures, packed: a few hundred KB for an 80x500
page) and keeps it with the file when you save, in the data folder (`stages/`), so the stages span every evening
you worked on it. A file changed by another program starts a new history. With fewer different pictures than
asked, fewer stages are written (never copies). Design: [docs/workstages-design.md](../workstages-design.md).
