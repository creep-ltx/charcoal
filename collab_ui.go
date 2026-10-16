package main

// Drawing together, the interface (the protocol is in collab.go): File > Join a
// drawing session and Host a drawing session, the session line above the status
// line (where, who, the chat), and the other users' cursors on the page.

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------- joining

type CollabJoinReq struct {
	server, nick, group, pass Field
	err                       string
}

func openCollabJoin() {
	a.endPlay()
	d := &CollabJoinReq{server: Field{max: 200}, nick: Field{max: collabNickRunes}, group: Field{max: collabNickRunes}, pass: Field{max: 100, secret: true}}
	d.server.set(prefs.CollabServer)
	d.nick.set(prefs.CollabNick)
	d.group.set(prefs.CollabGroup)
	d.server.enter, d.nick.enter, d.group.enter, d.pass.enter = d.ok, d.ok, d.ok, d.ok
	a.dialog = d
	ui.focus = &d.server
	if prefs.CollabServer != "" {
		ui.focus = &d.pass
	}
}

func (d *CollabJoinReq) ok() {
	if strings.TrimSpace(d.nick.String()) == "" {
		d.err = "Give a nick: the others see it by your cursor and in the chat"
		ui.focus = &d.nick
		return
	}
	if err := a.collabJoin(d.server.String(), d.nick.String(), d.group.String(), d.pass.String()); err != nil {
		d.err = err.Error()
		ui.focus = &d.server
		return
	}
	prefs.CollabServer, prefs.CollabNick, prefs.CollabGroup = strings.TrimSpace(d.server.String()), d.nick.String(), d.group.String()
	savePrefs()
	closeDialog()
}

func (d *CollabJoinReq) fields() []*Field { return []*Field{&d.server, &d.nick, &d.group, &d.pass} }

func (d *CollabJoinReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(600, 250), "Join a drawing session")
	y := in.y
	col := colAfter(90, "Server", "Nick", "Group", "Password")
	for i, f := range []struct {
		label, hint string
		fl          *Field
	}{{"Server", "The Moebius (or IcyDraw) server: host:port/path, as Moebius writes it; port 8000 when none", &d.server},
		{"Nick", "Your name in the session", &d.nick},
		{"Group", "Your group (Moebius shows it beside the nick; may be empty)", &d.group},
		{"Password", "The session's password (none: leave it empty). Never saved", &d.pass}} {
		fb.text(in.x, y+3, f.label, cText)
		w := in.w - col
		if i > 0 {
			w = min(w, digitsW(26)+8)
		}
		u.field(Rect{in.x + col, y, w, gadH}, f.fl, f.hint)
		y += gadH + 6
	}
	y += 4
	for _, l := range wrap("The page comes from the server in a new tab; what you draw goes to the others as you draw it. PC pages only, as in Moebius.", in.w) {
		fb.text(in.x, y, l, cDim)
		y += lineH
	}
	if d.err != "" {
		y += 4
		for _, l := range wrap(d.err, in.w) {
			fb.text(in.x, y, l, cOrange)
			y += lineH
		}
	}
	switch buttonRow(u, in, "Join", "Cancel") {
	case 0:
		d.ok()
	case 1:
		closeDialog()
	}
}

func (d *CollabJoinReq) key(k string, ctrl, shift bool) bool {
	return collabReqKey(k, shift, d.fields(), d.ok)
}

// collabReqKey: Tab and Shift+Tab go round the fields, Return is OK, Esc cancels.
func collabReqKey(k string, shift bool, order []*Field, ok func()) bool {
	switch k {
	case "Tab":
		for i, f := range order {
			if ui.focus == f {
				n := i + 1
				if shift {
					n = i + len(order) - 1
				}
				ui.focus = order[n%len(order)]
				return true
			}
		}
		ui.focus = order[0]
	case "Escape":
		closeDialog()
	case "Return", "Keypad Enter":
		ok()
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- hosting

type CollabHostReq struct {
	port, nick, pass Field
	all              bool
	err              string
}

func openCollabHost() {
	a.endPlay()
	d := &CollabHostReq{port: Field{max: 5}, nick: Field{max: collabNickRunes}, pass: Field{max: 100, secret: true}, all: prefs.CollabAll}
	port := prefs.CollabPort
	if port == 0 {
		port = collabPort
	}
	d.port.set(strconv.Itoa(port))
	d.nick.set(prefs.CollabNick)
	d.port.enter, d.nick.enter, d.pass.enter = d.ok, d.ok, d.ok
	a.dialog = d
	ui.focus = &d.pass
}

func (d *CollabHostReq) ok() {
	port, err := strconv.Atoi(strings.TrimSpace(d.port.String()))
	if err != nil {
		d.err = "The port is a number (Moebius's is 8000)"
		ui.focus = &d.port
		return
	}
	if d.all && d.pass.String() == "" {
		d.err = "Give a password when others on the network can join: anyone who finds the port could draw"
		ui.focus = &d.pass
		return
	}
	if err := a.collabHost(port, d.all, d.nick.String(), d.pass.String()); err != nil {
		d.err = "Could not host: " + err.Error()
		return
	}
	prefs.CollabPort, prefs.CollabAll = port, d.all
	if n := strings.TrimSpace(d.nick.String()); n != "" {
		prefs.CollabNick = n
	}
	savePrefs()
	closeDialog()
	a.msg("Hosting " + a.name + " on port " + strconv.Itoa(port) + ": " + collabHostWhere(port, d.all))
}

// collabHostWhere tells where others join.
func collabHostWhere(port int, all bool) string {
	if !all {
		return fmt.Sprintf("others on this computer join 127.0.0.1:%d", port)
	}
	addrs := collabAddrs()
	if len(addrs) == 0 {
		return fmt.Sprintf("others join <this computer's address>:%d", port)
	}
	for i := range addrs {
		addrs[i] = fmt.Sprintf("%s:%d", addrs[i], port)
	}
	return "others join " + strings.Join(addrs, " or ")
}

func (d *CollabHostReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(620, 285), "Host a drawing session")
	y := in.y
	col := colAfter(90, "Port", "Who", "Nick", "Password")
	fb.text(in.x, y+3, "Port", cText)
	u.field(Rect{in.x + col, y, digitsW(7) + 8, gadH}, &d.port, "The port to listen on: Moebius's is 8000")
	y += gadH + 6
	fb.text(in.x, y+3, "Who", cText)
	items := []string{"This computer only", "Others on the network"}
	cur := 0
	if d.all {
		cur = 1
	}
	u.choice("collabwho", Rect{in.x + col, y, choiceW(items), gadH}, items, cur,
		"This computer only: 127.0.0.1, nobody else can reach it. Others on the network: every address of this computer", func(i int) { d.all = i == 1 })
	y += gadH + 6
	fb.text(in.x, y+3, "Nick", cText)
	u.field(Rect{in.x + col, y, digitsW(26) + 8, gadH}, &d.nick, "Your name in the session")
	y += gadH + 6
	fb.text(in.x, y+3, "Password", cText)
	u.field(Rect{in.x + col, y, digitsW(26) + 8, gadH}, &d.pass, "The others need it to join. Never saved")
	y += gadH + 10
	port, _ := strconv.Atoi(strings.TrimSpace(d.port.String()))
	text := "Shares the page being worked on (" + a.name + "): Moebius (Network > Connect) and IcyDraw join it. While hosting, the line above the status line says so; File > End the drawing session stops it."
	if port > 0 {
		text += " " + strings.ToUpper(collabHostWhere(port, d.all)[:1]) + collabHostWhere(port, d.all)[1:] + "."
	}
	for _, l := range wrap(text, in.w) {
		fb.text(in.x, y, l, cDim)
		y += lineH
	}
	if d.err != "" {
		y += 4
		for _, l := range wrap(d.err, in.w) {
			fb.text(in.x, y, l, cOrange)
			y += lineH
		}
	}
	switch buttonRow(u, in, "Host", "Cancel") {
	case 0:
		d.ok()
	case 1:
		closeDialog()
	}
}

func (d *CollabHostReq) key(k string, ctrl, shift bool) bool {
	return collabReqKey(k, shift, []*Field{&d.port, &d.nick, &d.pass}, d.ok)
}

// collabEndKey ends the session (File > End the drawing session).
func collabEndKey() {
	s := collab
	switch {
	case s == nil:
		a.msg("No drawing session is going on")
	case s.host:
		a.collabEnd("Stopped hosting: nobody can join any more, the page stays")
	default:
		a.collabEnd("Left the drawing session: the page stays here, not saved")
	}
}

// collabChatKey puts the typing into the chat field.
func collabChatKey() {
	s := collab
	if s == nil || !s.ready {
		a.msg("No drawing session: File > Join a drawing session or Host a drawing session")
		return
	}
	s.chatField.enter = func() {
		a.collabSay(s.chatField.String())
		s.chatField.set("")
	}
	s.chatField.max = collabChatRunes
	ui.focus = &s.chatField
	s.unread = 0
}

// ---------------------------------------------------------------- on screen

// collabLineH is the height of the session line above the status line (0 without a session).
func collabLineH() int {
	if collab == nil || a.bare {
		return 0
	}
	return lineH + 8
}

// drawCollabLine: where the session is, who is in it, the last chat line and the chat field.
func (a *App) drawCollabLine(u *UI, r Rect) {
	s := collab
	fb := u.fb
	fb.fill(Rect{r.x, r.y, r.w, 1}, cEdgeDk)
	fb.fill(Rect{r.x, r.y + 1, r.w, r.h - 1}, cDark)
	y := r.y + 4
	x := r.x + 8
	where := "Drawing at " + s.addr
	switch {
	case s.host:
		where = "HOSTING on port " + strings.TrimPrefix(s.addr, "port ")
		if s.all {
			where += " for the network"
		} else {
			where += " for this computer"
		}
		lr := Rect{x - 4, r.y + 2, textW(where) + 8, r.h - 3}
		fb.fill(lr, cOrange)
		x = fb.text(x, y, where, cDark) + 16
		u.setHint(lr, "Hosting: "+collabHostWhere(s.portNum(), s.all)+". File > End the drawing session stops it")
	case !s.ready:
		x = fb.text(x, y, "Joining "+s.addr+" ...", cOrange) + 16
	default:
		x = fb.text(x, y, where, cText) + 16
	}
	if s.tab != a.Tab {
		x = fb.text(x, y, "(page \""+s.tab.name+"\")", cDim) + 16
	}
	if s.ready {
		x = fb.text(x, y, a.collabWho(s), cText) + 24
	}
	fw := min(420, max(160, r.w/3))
	fr := Rect{r.x + r.w - fw - 8, r.y + 1, fw, r.h - 2}
	if s.ready {
		// The last chat line, between who and the field.
		if n := len(s.chat); n > 0 && ui.focus != &s.chatField {
			l := s.chat[n-1]
			text := l.text
			if l.nick != "" {
				text = l.nick + ": " + text
			}
			col := uint32(cDim)
			if time.Since(l.at) < 30*time.Second || s.unread > 0 {
				col = cText
			}
			room := fr.x - 16 - x
			if room > 40 {
				rs := []rune(text)
				fb.text(x, y, string(rs[:fitText(rs, room)]), col)
			}
		}
		if ui.focus != &s.chatField && len(s.chatField.text) == 0 {
			collabChatKeyHint := "Chat (" + keyName("collab_chat") + ")"
			if keyName("collab_chat") == "" {
				collabChatKeyHint = "Chat"
			}
			fb.fill(fr.inset(2), cGrey)
			fb.text(fr.x+8, y, collabChatKeyHint, cDim)
			if c := u.take(fr); c != nil {
				collabChatKey()
			}
		} else {
			s.chatField.enter = func() {
				a.collabSay(s.chatField.String())
				s.chatField.set("")
			}
			u.field(fr, &s.chatField, "Chat with the others: Return sends, Esc goes back to the page")
		}
		if ui.focus == &s.chatField {
			s.unread = 0
			a.drawCollabChat(u, Rect{fr.x, r.y, fr.w, 0})
		}
	}
}

// drawCollabChat shows the last chat lines over the page, above the chat field, while it has the keys.
func (a *App) drawCollabChat(u *UI, at Rect) {
	s := collab
	fb := u.fb
	n := min(len(s.chat), 8)
	if n == 0 {
		return
	}
	var lines []string
	for _, l := range s.chat[len(s.chat)-n:] {
		text := l.text
		if l.nick != "" {
			text = l.nick + ": " + text
		}
		lines = append(lines, wrap(text, at.w-12)...)
	}
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	h := len(lines)*lineH + 8
	box := Rect{at.x, at.y - h, at.w, h}
	fb.fill(box, cDark)
	fb.frame(box, cEdgeDk)
	for i, l := range lines {
		fb.text(box.x+6, box.y+4+i*lineH, l, cText)
	}
}

func (s *collabSession) portNum() int {
	n, _ := strconv.Atoi(strings.TrimPrefix(s.addr, "port "))
	return n
}

// collabColours tell the users' cursors apart.
var collabColours = []uint32{0xFF55FFFF, 0xFFFF55FF, 0xFF55FF55, 0xFFFFFF55, 0xFF5599FF, 0xFFFF9955}

// drawCollabCursors frames the other users' cursors on the page, with their nick above.
func (a *App) drawCollabCursors(fb *FB, cell func(x, y, w, h int) Rect) {
	s := collab
	if s == nil || !s.ready || s.tab != a.Tab {
		return
	}
	for _, u := range s.collabOthers() {
		if !u.cursor || u.web || u.x >= a.doc.cols || u.y >= a.doc.rows {
			continue
		}
		col := collabColours[u.id%len(collabColours)]
		r := cell(u.x, u.y, 1, 1)
		fb.frame(r, col)
		fb.frame(r.inset(1), 0xFF000000)
		name := u.nick
		if name == "" {
			name = "Guest"
		}
		w := textW(name) + 6
		lr := Rect{r.x, r.y - lineH - 2, w, lineH + 2}
		if lr.y < fb.clip.y {
			lr.y = r.y + r.h
		}
		fb.blend(lr, col, 200)
		fb.text(lr.x+3, lr.y+1, name, 0xFF000000)
	}
}
