[Charcoal](../../README.md) · [Manual](README.md)

# Installing Charcoal

## Download

What is new in each version: [CHANGELOG.md](../../CHANGELOG.md) (each release page says the same).

Get the newest version from the [releases page](https://github.com/creep-ltx/charcoal/releases): open
"Assets" under the top release and pick the zip for your computer. Unpack it (double-click the zip). Each
holds the program, the README, this manual, the licence and the font and library licences (`licences/`).

- **Windows** (10 or 11, 64-bit): `charcoal-windows-x64.zip`. Double-click `charedit.exe` to draw;
  `charview.exe` is the viewer (drop files or a folder on it). The first time, Windows may say
  *"Windows protected your PC"*, because the program is not signed: click **More info**, then **Run anyway**.
- **Mac** (macOS 11 or newer, Apple Silicon and Intel): `charcoal-macos.zip` holds **Charcoal.app**. Drag it to
  Applications. It is not signed by Apple, so the first double-click says it *"cannot be opened"* or that
  Apple *"could not verify"* it: click **Done**, open **System Settings > Privacy & Security**, scroll down and
  click **Open Anyway** next to Charcoal, then **Open**. (If it says the app *"is damaged"*, or there is no Open
  Anyway, type this in Terminal instead: `xattr -dr com.apple.quarantine /Applications/Charcoal.app`.) After that it opens like any program. Drop art files on its icon to open them.
  On a Mac, **Cmd** works wherever these pages say Ctrl, Alt is the **left** Option key, and the **right**
  Option key types characters such as @ { [ | \. The viewer: `Charcoal.app/Contents/MacOS/charedit --view FILE`
  in Terminal.
- **Linux** (64-bit PC): `charcoal-linux-x64.zip`. It needs SDL2, which most desktops already have (if not:
  `sudo apt install libsdl2-2.0-0`, `sudo pacman -S sdl2` or `sudo dnf install SDL2`). Run `./charedit`;
  `charview` is a link to it.



## Where settings are kept

Settings, keys and autosaves are kept in `%AppData%\Charcoal` on Windows,
`~/Library/Application Support/Charcoal` on a Mac and `~/.config/charcoal` + `~/.local/share/charcoal` on Linux.

Settings are in `prefs.json` there (`"uiscale": 2` doubles the window for big screens); the keys in `keys` (see [Keys](keys.md)).

## Building it yourself

See [Building and the source](building.md).
