# make               builds charedit (needs Go and the SDL2 development files)
# make install       charedit and charview (a link to it) to $(PREFIX)/bin, plus the menu entries and icon
# make uninstall     removes what install put there
# make install PREFIX=$HOME/.local installs for one user without root; DESTDIR is for packagers.
# charedit is rebuilt only when a source changes, so a sudo make install after make does not build as root.

PREFIX  ?= /usr/local
BINDIR  ?= $(PREFIX)/bin
DATADIR ?= $(PREFIX)/share
GO      ?= go

all: charedit

charedit: $(wildcard *.go) $(wildcard fonts/*) go.mod go.sum
	$(GO) build -ldflags '-s -w' -o charedit .

test:
	$(GO) test ./...

install: charedit
	install -Dm755 charedit $(DESTDIR)$(BINDIR)/charedit
	ln -sf charedit $(DESTDIR)$(BINDIR)/charview
	install -Dm644 tools/desktop/charedit.desktop $(DESTDIR)$(DATADIR)/applications/charedit.desktop
	install -Dm644 tools/desktop/charview.desktop $(DESTDIR)$(DATADIR)/applications/charview.desktop
	install -Dm644 tools/desktop/charcoal.svg $(DESTDIR)$(DATADIR)/icons/hicolor/scalable/apps/charcoal.svg
	-[ -n "$(DESTDIR)" ] || update-desktop-database -q $(DATADIR)/applications 2>/dev/null
	-[ -n "$(DESTDIR)" ] || gtk-update-icon-cache -qtf $(DATADIR)/icons/hicolor 2>/dev/null

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/charedit $(DESTDIR)$(BINDIR)/charview
	rm -f $(DESTDIR)$(DATADIR)/applications/charedit.desktop $(DESTDIR)$(DATADIR)/applications/charview.desktop
	rm -f $(DESTDIR)$(DATADIR)/icons/hicolor/scalable/apps/charcoal.svg

clean:
	rm -f charedit charcoal

.PHONY: all test install uninstall clean
