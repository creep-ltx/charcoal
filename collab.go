package main

// Drawing together: CharEdit joins a drawing session on a Moebius server (or
// IcyDraw's), or hosts one that Moebius and IcyDraw can join. The protocol is
// Moebius's (app/server.js, app/document/doc.js in blocktronics/moebius): JSON
// messages {"type": n, "data": {...}} over a websocket; a cell is {code, fg, bg}
// with a CP437 code and colours in BIOS order; the page comes as the runs of
// libtextmode.compress. The user interface is in collab_ui.go.
//
// Everything that comes over the network is untrusted: sizes, cells and strings
// are checked before they touch a page, a message that does not fit is dropped,
// and a guest's id is never taken from what it says.
//
// How edits travel: the session keeps the page as the others have it (shadow).
// Each pass of the main loop first applies what came in (to the page and the
// shadow), then sends every cell where the page and the shadow differ. So every
// way of changing the page (typing, tools, undo, paste, effects) is shared without
// a hook in each of them; the same goes for the size, iCE colours, 9-pixel cells,
// the font and the SAUCE text.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/coder/websocket"
)

// Moebius's message types (app/server.js `action`, app/document/doc.js `actions`).
const (
	cbConnected = iota
	cbRefused
	cbJoin
	cbLeave
	cbCursor
	cbSelection
	cbResizeSelection
	cbOperation
	cbHideCursor
	cbDraw
	cbChat
	cbStatus
	cbSauce
	cbIce
	cbNinePx
	cbFont
	cbCanvasSize
	cbPasteAsSelection
	cbRotate
	cbFlipX
	cbFlipY
	cbSetBg
)

// A user's status (Moebius's status_types); web is a viewer without a nick.
const (
	cbActive = iota
	cbIdle
	cbAway
	cbWeb
)

// Limits on what the network may hand CharEdit.
const (
	collabMaxCells    = 2 << 20  // a shared page's cells (Moebius pages are 80 x a few thousand)
	collabMaxCols     = 1000     // as setCols
	collabDocBytes    = 48 << 20 // the page as it comes on joining: 2M cells of runs that never repeat
	collabGuestBytes  = 16 << 20 // one message from a guest (a pasted block being placed)
	collabMaxGuests   = 32
	collabChatRunes   = 1000
	collabNickRunes   = 32
	collabCommentRune = 64 * 255 // SAUCE comment lines
	collabChatKept    = 32       // chat lines kept, as Moebius's server
	collabPort        = 8000     // Moebius's default port
)

type cbMsg struct {
	Type int             `json:"type"`
	Data json.RawMessage `json:"data"`
}

type cbBlock struct {
	Code int `json:"code"`
	Fg   int `json:"fg"`
	Bg   int `json:"bg"`
}

type cbRGB struct {
	R int `json:"r"`
	G int `json:"g"`
	B int `json:"b"`
}

type cbRuns struct {
	Code [][2]int `json:"code"`
	Fg   [][2]int `json:"fg"`
	Bg   [][2]int `json:"bg"`
}

// cbDoc is a page as libtextmode.compress gives it.
type cbDoc struct {
	Columns    int       `json:"columns"`
	Rows       int       `json:"rows"`
	Title      string    `json:"title"`
	Author     string    `json:"author"`
	Group      string    `json:"group"`
	Date       string    `json:"date,omitempty"`
	Palette    []cbRGB   `json:"palette"`
	FontName   string    `json:"font_name"`
	Ice        bool      `json:"ice_colors"`
	NinePx     bool      `json:"use_9px_font"`
	Comments   string    `json:"comments"`
	Compressed *cbRuns   `json:"compressed_data,omitempty"`
	Data       []cbBlock `json:"data,omitempty"` // a page sent without runs (IcyDraw reads both)
}

type cbUserJSON struct {
	ID     int     `json:"id"`
	Nick   *string `json:"nick,omitempty"` // none: a web viewer
	Group  string  `json:"group"`
	Status int     `json:"status"`
}

type cbChatJSON struct {
	ID    int    `json:"id"`
	Nick  string `json:"nick"`
	Group string `json:"group"`
	Text  string `json:"text"`
	Time  int64  `json:"time,omitempty"`
}

type cbBlocks struct {
	Columns int       `json:"columns"`
	Rows    int       `json:"rows"`
	Data    []cbBlock `json:"data"`
}

// cbIn is any message's data: each type uses some of the fields.
type cbIn struct {
	ID          int             `json:"id"`
	Nick        *string         `json:"nick"`
	Group       string          `json:"group"`
	Pass        string          `json:"pass"`
	Status      int             `json:"status"`
	X           int             `json:"x"`
	Y           int             `json:"y"`
	Block       *cbBlock        `json:"block"`
	Blocks      *cbBlocks       `json:"blocks"`
	Text        string          `json:"text"`
	Time        int64           `json:"time"`
	Title       string          `json:"title"`
	Author      string          `json:"author"`
	Comments    string          `json:"comments"`
	Value       json.RawMessage `json:"value"`
	FontName    string          `json:"font_name"`
	Columns     int             `json:"columns"`
	Rows        int             `json:"rows"`
	Doc         *cbDoc          `json:"doc"`
	Users       []cbUserJSON    `json:"users"`
	ChatHistory []cbChatJSON    `json:"chat_history"`
}

// ---------------------------------------------------------------- cells

// cbCell is the PC cell of a Moebius block; false for a block no cell can be.
// A code past 255 (a Unicode character) is its CP437 code, or "?".
func cbCell(b cbBlock) (uint32, bool) {
	if b.Fg < 0 || b.Fg > 15 || b.Bg < 0 || b.Bg > 15 || b.Code < 0 || b.Code > unicode.MaxRune {
		return 0, false
	}
	code := b.Code
	if code > 255 {
		code = cp437Code(rune(code))
	}
	return makeCell(code, biosANSI[b.Fg], biosANSI[b.Bg], 0), true
}

func cbBlockOf(v uint32) cbBlock {
	return cbBlock{Code: cellCh(v), Fg: biosANSI[cellFg(v)], Bg: biosANSI[cellBg(v)]}
}

// cbUnknown marks a shadow cell whose value the others' pages may not have: the
// new cells of a page made larger here (IcyDraw's server fills them with code 0
// on black, Moebius's with a space on grey), sent in full.
const cbUnknown = 1 << 31

// cbSame: page cell a is shadow cell b (italic or underline bits do not travel).
func cbSame(a, b uint32) bool { return b&cbUnknown == 0 && a&0xFFFF == b&0xFFFF }

var cp437Codes = sync.OnceValue(func() map[rune]int {
	m := make(map[rune]int, 256)
	for i := 255; i >= 0; i-- {
		m[cp437[i]] = i
	}
	return m
})

func cp437Code(r rune) int {
	if c, ok := cp437Codes()[r]; ok {
		return c
	}
	return '?'
}

// cbExpand undoes one stream of runs ([value, repeats]: repeats more after the
// first), refusing values outside lo-hi and anything that is not exactly n long.
func cbExpand(runs [][2]int, n, lo, hi int) ([]int, error) {
	out := make([]int, 0, n)
	for _, r := range runs {
		if r[0] < lo || r[0] > hi {
			return nil, fmt.Errorf("a value of %d", r[0])
		}
		if r[1] < 0 || r[1] >= n-len(out) {
			return nil, errors.New("runs longer than the page")
		}
		for range r[1] + 1 {
			out = append(out, r[0])
		}
	}
	if len(out) != n {
		return nil, errors.New("runs shorter than the page")
	}
	return out, nil
}

func cbRunsOf(v []int) [][2]int {
	var out [][2]int
	for i := 0; i < len(v); {
		j := i + 1
		for j < len(v) && v[j] == v[i] {
			j++
		}
		out = append(out, [2]int{v[i], j - i - 1})
		i = j
	}
	return out
}

// collabSizeOK: a page this size may be shared.
func collabSizeOK(cols, rows int) bool {
	return cols >= 1 && cols <= collabMaxCols && rows >= 1 && rows <= maxRowsFor(cols) && cols*rows <= collabMaxCells
}

// docOf turns the page that came on joining into a Doc; it checks every number.
func (d *cbDoc) docOf() (*Doc, error) {
	if !collabSizeOK(d.Columns, d.Rows) {
		return nil, fmt.Errorf("a page of %dx%d is too large to share", d.Columns, d.Rows)
	}
	n := d.Columns * d.Rows
	doc := newDoc(d.Columns, d.Rows)
	switch {
	case d.Compressed != nil:
		code, err := cbExpand(d.Compressed.Code, n, 0, unicode.MaxRune)
		if err != nil {
			return nil, fmt.Errorf("the page's characters: %v", err)
		}
		fg, err := cbExpand(d.Compressed.Fg, n, 0, 15)
		if err != nil {
			return nil, fmt.Errorf("the page's colours: %v", err)
		}
		bg, err := cbExpand(d.Compressed.Bg, n, 0, 15)
		if err != nil {
			return nil, fmt.Errorf("the page's backgrounds: %v", err)
		}
		for i := range n {
			doc.cells[i], _ = cbCell(cbBlock{code[i], fg[i], bg[i]})
		}
	case len(d.Data) == n:
		for i, b := range d.Data {
			v, ok := cbCell(b)
			if !ok {
				return nil, errors.New("a cell that cannot be")
			}
			doc.cells[i] = v
		}
	default:
		return nil, errors.New("no cells")
	}
	return doc, nil
}

// cbPalette is a page's palette from the wire (Moebius sends 0-63, IcyDraw may
// send 0-255) in the cells' order; nil for the standard one or one that makes no sense.
func cbPalette(p []cbRGB) *[16]uint32 {
	if len(p) != 16 {
		return nil
	}
	six := true
	for _, c := range p {
		for _, v := range []int{c.R, c.G, c.B} {
			if v < 0 || v > 255 {
				return nil
			}
			six = six && v <= 63
		}
	}
	var pal [16]uint32
	std := true
	for i, c := range p {
		r, g, b := uint32(c.R), uint32(c.G), uint32(c.B)
		if six {
			r, g, b = r<<2|r>>4, g<<2|g>>4, b<<2|b>>4
		}
		pal[i] = 0xFF000000 | r<<16 | g<<8 | b
		std = std && pal[i] == cbEGA[i]
	}
	if std {
		return nil
	}
	pal = biosPalette(pal)
	return &pal
}

// cbEGA: Moebius's standard palette (libtextmode/palette.js ega) at 8 bits, BIOS order.
var cbEGA = [16]uint32{0xFF000000, 0xFF0000AA, 0xFF00AA00, 0xFF00AAAA, 0xFFAA0000, 0xFFAA00AA, 0xFFAA5500, 0xFFAAAAAA,
	0xFF555555, 0xFF5555FF, 0xFF55FF55, 0xFF55FFFF, 0xFFFF5555, 0xFFFF55FF, 0xFFFFFF55, 0xFFFFFFFF}

// clean makes a string from the network fit to show: no control characters (but the
// line breaks of SAUCE comments), at most n runes.
func clean(s string, n int) string {
	var b strings.Builder
	k := 0
	for _, r := range s {
		if k >= n {
			break
		}
		if unicode.IsControl(r) && (r != '\n' || n != collabCommentRune) {
			continue
		}
		b.WriteRune(r)
		k++
	}
	return b.String()
}

// ---------------------------------------------------------------- connections

// cbConn is one websocket: a goroutine reads messages into the session's
// channel, another writes what the main loop queues.
type cbConn struct {
	ws     *websocket.Conn
	out    chan []byte
	done   chan struct{}
	once   sync.Once
	ip     string
	in     bool        // logged in (host side: CONNECTED was answered)
	login  atomic.Bool // the reader's copy, for the login deadline
	user   *collabUser // host side: the guest it is (main loop only)
	reason string
}

// cbEvent is what a connection hands the main loop: a message, the connection
// made (up), or its end (err).
type cbEvent struct {
	c   *cbConn
	msg cbMsg
	up  bool
	err error
}

func newCBConn(ws *websocket.Conn, ip string, limit int64, in chan cbEvent) *cbConn {
	ws.SetReadLimit(limit)
	c := &cbConn{ws: ws, out: make(chan []byte, 1<<16), done: make(chan struct{}), ip: ip}
	go c.readLoop(in)
	go c.writeLoop()
	return c
}

func (c *cbConn) readLoop(in chan cbEvent) {
	for {
		typ, b, err := c.ws.Read(context.Background())
		if err != nil {
			select {
			case in <- cbEvent{c: c, err: err}:
			case <-c.done:
			}
			c.close("")
			return
		}
		var m cbMsg
		if typ != websocket.MessageText || json.Unmarshal(b, &m) != nil {
			continue // not a message of the protocol: dropped
		}
		select {
		case in <- cbEvent{c: c, msg: m}:
		case <-c.done:
			return
		}
	}
}

func (c *cbConn) writeLoop() {
	for {
		select {
		case b := <-c.out:
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			err := c.ws.Write(ctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				c.close("")
				return
			}
		case <-c.done:
			return
		}
	}
}

// send queues a message; a connection that cannot keep up is closed.
func (c *cbConn) send(typ int, data any) {
	b, err := json.Marshal(struct {
		Type int `json:"type"`
		Data any `json:"data"`
	}{typ, data})
	if err != nil {
		return
	}
	select {
	case <-c.done:
	case c.out <- b:
	default:
		c.close("too slow")
	}
}

func (c *cbConn) close(reason string) {
	c.once.Do(func() {
		c.reason = reason
		close(c.done)
		if c.ws != nil {
			go c.ws.Close(websocket.StatusNormalClosure, "")
		}
	})
}

// closeAfter closes the connection once what is queued has gone (or after a second).
func (c *cbConn) closeAfter() {
	go func() {
		for i := 0; i < 20 && len(c.out) > 0; i++ {
			time.Sleep(50 * time.Millisecond)
		}
		time.Sleep(100 * time.Millisecond)
		c.close("")
	}()
}

// ---------------------------------------------------------------- the session

type collabUser struct {
	id          int
	nick, group string
	web         bool // a viewer without a nick (Moebius's web page)
	status      int
	x, y        int
	cursor      bool // its cursor is on the page
	sel         bool // it is selecting (or placing a block)
	conn        *cbConn
}

type collabLine struct {
	id         int
	nick, text string
	at         time.Time
}

type collabSession struct {
	host              bool
	tab               *Tab
	addr              string // the server joined, or "port 8000"
	nick, group, pass string
	id                int // ours in the session
	users             map[int]*collabUser
	chat              []collabLine
	in                chan cbEvent
	conn              *cbConn // joined: the server
	ready             bool    // joined: the page came
	ln                net.Listener
	srv               *http.Server
	all               bool // hosted on every address, not only this computer
	nextID            int
	hostChat          []cbChatJSON // hosting: the chat a guest gets on joining
	guests            atomic.Int32
	cancel            context.CancelFunc
	// The page as the others have it.
	shadow       *Doc
	ice, ninePx  bool
	font         string
	info         Info
	cx, cy       int
	cursorShown  bool
	chatField    Field
	unread       int
	since        time.Time
	sentCells    int // counters for the tests and the status hint
	gotCells     int
	refused      bool
	lastActivity time.Time
}

// collab is the session going on (one at a time), nil when none.
var collab *collabSession

// collabEnded says why the last session ended, for the status line.
func (a *App) collabEnd(why string) {
	s := collab
	if s == nil {
		return
	}
	collab = nil
	if s.cancel != nil {
		s.cancel()
	}
	if s.conn != nil {
		s.conn.close("")
	}
	for _, u := range s.users {
		if u.conn != nil {
			u.conn.close("")
		}
	}
	if s.srv != nil {
		s.srv.Close()
	}
	if s.tab != nil && a.tabIndex(s.tab) >= 0 && !s.host && s.ready {
		s.tab.dirty = true // the page stays, as a page of its own: closing it asks to save
	}
	if ui.focus == &s.chatField {
		ui.focus = nil
	}
	if why != "" {
		a.msg(why)
	}
}

// collabActive: a session is going on (the main loop then looks often for messages).
func collabActive() bool { return collab != nil }

// collabJoin starts joining a server at addr ("host[:port][/path]", Moebius's form,
// or a ws:// or wss:// address) in a new tab.
func (a *App) collabJoin(addr, nick, group, pass string) error {
	u, err := collabURL(addr)
	if err != nil {
		return err
	}
	if collab != nil {
		a.collabEnd("")
	}
	s := &collabSession{addr: strings.TrimPrefix(strings.TrimPrefix(u, "ws://"), "wss://"), nick: clean(nick, collabNickRunes),
		group: clean(group, collabNickRunes), pass: pass, in: make(chan cbEvent, 4096), users: map[int]*collabUser{}, since: time.Now()}
	was := a.Tab
	t := a.newTab("pc")
	t.name = collabFileName(u)
	t.saveFmt = "ansi"
	a.focusTab(t)
	a.start = false
	if was != nil && was.untouched() && was.doc.empty() { // the empty page (start screen) gives way, as a tile picked there
		a.closeTab(was)
		a.focusTab(t)
	}
	s.tab = t
	collab = s
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		dctx, done := context.WithTimeout(ctx, 15*time.Second)
		defer done()
		ws, _, err := websocket.Dial(dctx, u, nil)
		if err != nil {
			select {
			case s.in <- cbEvent{err: err}:
			case <-ctx.Done():
			}
			return
		}
		c := newCBConn(ws, "", collabDocBytes, s.in)
		select {
		case s.in <- cbEvent{c: c, up: true}:
		case <-ctx.Done():
			c.close("")
		}
	}()
	a.msg("Joining " + s.addr + " ...")
	return nil
}

// collabFileName is the name a joined page is saved under: the session's path (Moebius names it after the served file).
func collabFileName(u string) string {
	p, err := url.Parse(u)
	name := ""
	if err == nil {
		name = strings.Trim(p.Path, "/")
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		if name == "" {
			name = p.Hostname()
		}
	}
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '_'
		}
		return r
	}, name)
	if name == "" {
		name = "session"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".ans") {
		name += ".ans"
	}
	return name
}

// collabURL makes the websocket address of what the user typed: Moebius's
// "host:port/path" (port 8000 when none) or a ws:// / wss:// address.
func collabURL(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("no server given")
	}
	if !strings.HasPrefix(s, "ws://") && !strings.HasPrefix(s, "wss://") {
		if strings.Contains(s, "://") {
			return "", errors.New("a drawing session's address starts ws:// or wss://, or is host:port/path")
		}
		s = "ws://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Hostname() == "" {
		return "", errors.New("not a server address: " + s)
	}
	if u.Port() == "" {
		u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(collabPort))
	}
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

// collabHost shares the page being worked on: a server on the port, on this
// computer only (127.0.0.1) unless all, which Moebius and IcyDraw can join.
func (a *App) collabHost(port int, all bool, nick, pass string) error {
	switch {
	case !a.pc():
		return errors.New("only a PC page can be shared: Moebius and IcyDraw draw PC pages")
	case a.frames != nil:
		return errors.New("a page with frames cannot be shared: Moebius pages are one picture")
	case a.layers != nil:
		return errors.New("a page of layers cannot be shared: Moebius pages are one picture (merge the layers first)")
	case !collabSizeOK(a.doc.cols, a.doc.rows):
		return fmt.Errorf("the page is too large to share (%dx%d)", a.doc.cols, a.doc.rows)
	case port < 1 || port > 65535:
		return errors.New("the port is 1-65535")
	}
	host := "127.0.0.1"
	if all {
		host = ""
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	if collab != nil {
		a.collabEnd("")
	}
	s := &collabSession{host: true, tab: a.Tab, addr: fmt.Sprintf("port %d", port), nick: clean(nick, collabNickRunes), pass: pass,
		in: make(chan cbEvent, 4096), users: map[int]*collabUser{}, ln: ln, all: all, nextID: 1, ready: true, since: time.Now()}
	if s.nick == "" {
		s.nick = "host"
	}
	s.users[0] = &collabUser{id: 0, nick: s.nick}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.srv = &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.guests.Add(1) > collabMaxGuests {
			s.guests.Add(-1)
			http.Error(w, "full", http.StatusServiceUnavailable)
			return
		}
		// Moebius's Electron page sends its own origin: any origin is let in (the password guards the page).
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			s.guests.Add(-1)
			return
		}
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		c := newCBConn(ws, ip, collabGuestBytes, s.in)
		go func() { // a guest that does not log in within 30 s is let go
			select {
			case <-time.After(30 * time.Second):
				if !c.login.Load() {
					c.close("no login")
				}
			case <-c.done:
			}
		}()
		go func() {
			<-c.done
			s.guests.Add(-1)
		}()
		select {
		case <-ctx.Done():
			c.close("")
		case <-c.done:
		}
	})}
	go s.srv.Serve(ln)
	s.takeShadow(a)
	collab = s
	return nil
}

// takeShadow: the page as it is now is what the others have.
func (s *collabSession) takeShadow(a *App) {
	t := s.tab
	s.shadow = t.doc.clone()
	s.ice, s.ninePx = t.ice, t.ninePx()
	s.font = a.withTab(t, a.sauceFont)
	s.info = t.info
	s.cx, s.cy = -1, -1
}

// withTab runs f with t as the tab being worked on.
func (a *App) withTab(t *Tab, f func() string) string {
	old := a.Tab
	a.Tab = t
	defer func() { a.Tab = old }()
	return f()
}

// ---------------------------------------------------------------- each pass

// collabTick applies what came in and sends what changed here; true when the window should be drawn again.
func (a *App) collabTick() bool {
	s := collab
	if s == nil {
		return false
	}
	if a.tabIndex(s.tab) < 0 {
		a.collabEnd("The page of the drawing session was closed: the session ended")
		return true
	}
	changed := false
	for n := 0; n < 20000 && collab == s; n++ {
		select {
		case ev := <-s.in:
			a.collabEvent(s, ev)
			changed = true
			continue
		default:
		}
		break
	}
	if collab != s || !s.ready {
		return true
	}
	if s.tab.frames != nil {
		a.collabEnd("Frames cannot be shared (Moebius pages are one picture): the drawing session ended")
		return true
	}
	if s.tab.layers != nil {
		a.collabEnd("Layers cannot be shared (Moebius pages are one picture): the drawing session ended")
		return true
	}
	if s.tab.mode != "pc" {
		a.collabEnd("The page is no longer a PC page: the drawing session ended")
		return true
	}
	a.collabSendChanges(s)
	return changed
}

// collabEvent handles one thing from the network.
func (a *App) collabEvent(s *collabSession, ev cbEvent) {
	if s.host {
		a.collabGuestEvent(s, ev)
		return
	}
	switch {
	case ev.err != nil && (ev.c == nil || ev.c == s.conn):
		why := "The drawing session ended: the server closed the connection"
		switch {
		case s.refused:
			why = "The server let you in no further: wrong password?"
		case !s.ready && ev.c == nil:
			why = "Could not join " + s.addr + ": " + collabErr(ev.err)
		case !s.ready:
			why = "Could not join " + s.addr + ": the server closed the connection"
		}
		a.collabEnd(why)
		return
	case ev.up:
		s.conn = ev.c
		s.conn.send(cbConnected, map[string]string{"nick": s.nick, "group": s.group, "pass": s.pass})
		return
	case ev.c != s.conn:
		return
	}
	var d cbIn
	if json.Unmarshal(ev.msg.Data, &d) != nil && ev.msg.Type != cbRefused {
		return // not what the type needs: dropped
	}
	if !s.ready {
		switch ev.msg.Type {
		case cbConnected:
			a.collabJoined(s, d)
		case cbRefused:
			s.refused = true
			a.collabEnd("The server let you in no further: wrong password?")
		}
		return
	}
	u := s.users[d.ID]
	switch ev.msg.Type {
	case cbJoin:
		if d.ID != s.id && len(s.users) < 4096 {
			s.users[d.ID] = cbUserOf(cbUserJSON{ID: d.ID, Nick: d.Nick, Group: d.Group, Status: d.Status})
			if d.Nick != nil {
				s.note(-1, "", clean(*d.Nick, collabNickRunes)+" joined")
			}
		}
	case cbLeave:
		if u != nil && d.ID != s.id {
			delete(s.users, d.ID)
			if !u.web {
				s.note(-1, "", u.nick+" left")
			}
		}
	case cbChat:
		if u != nil {
			s.note(d.ID, u.nick, clean(d.Text, collabChatRunes))
		}
	default:
		a.collabApply(s, u, ev.msg.Type, d)
	}
}

// collabErr is a network error in a few words.
func collabErr(err error) string {
	var op *net.OpError
	switch {
	case errors.As(err, &op) && op.Op == "dial":
		return "no server answers there"
	case errors.Is(err, context.DeadlineExceeded):
		return "no answer in 15 seconds"
	}
	s := err.Error()
	if i := strings.LastIndex(s, ": "); i >= 0 && len(s)-i < 60 {
		s = s[i+2:]
	}
	return s
}

func cbUserOf(j cbUserJSON) *collabUser {
	u := &collabUser{id: j.ID, group: clean(j.Group, collabNickRunes), status: j.Status, web: j.Nick == nil}
	if j.Nick != nil {
		u.nick = clean(*j.Nick, collabNickRunes)
	}
	return u
}

// collabJoined: the server sent the page (CONNECTED); it becomes the tab's.
func (a *App) collabJoined(s *collabSession, d cbIn) {
	if d.Doc == nil {
		a.collabEnd("Could not join " + s.addr + ": the server sent no page")
		return
	}
	doc, err := d.Doc.docOf()
	if err != nil {
		a.collabEnd("Could not join " + s.addr + ": " + err.Error())
		return
	}
	t := s.tab
	old := a.Tab
	a.Tab = t
	t.doc, t.ice = doc, d.Doc.Ice
	t.source, t.sauce, t.frames, t.undo, t.redo = nil, nil, nil, nil, nil
	t.sauceFlags = 0
	if d.Doc.NinePx {
		t.sauceFlags = 4
	}
	t.info = Info{writeSauce: true, title: sauceField(d.Doc.Title, 35), author: sauceField(d.Doc.Author, 20), group: sauceField(d.Doc.Group, 20),
		notes: collabNotes(d.Doc.Comments)}
	a.collabFont(sauceField(d.Doc.FontName, 64))
	if p := cbPalette(d.Doc.Palette); p != nil {
		t.ownPal, t.paletteFor["pc"] = p, "file"
	}
	t.cx, t.cy = 0, 0
	a.applyMode()
	a.gen = nextGen()
	a.markSaved()
	a.Tab = old
	if old == t {
		a.applyMode()
	}
	s.id = d.ID
	s.ready = true
	for _, j := range d.Users {
		if j.ID != s.id && len(s.users) < 4096 {
			s.users[j.ID] = cbUserOf(j)
		}
	}
	s.users[s.id] = &collabUser{id: s.id, nick: s.nick, group: s.group}
	for _, c := range d.ChatHistory {
		s.chat = append(s.chat, collabLine{id: c.ID, nick: clean(c.Nick, collabNickRunes), text: clean(c.Text, collabChatRunes), at: time.UnixMilli(c.Time)})
	}
	if len(s.chat) > collabChatKept {
		s.chat = s.chat[len(s.chat)-collabChatKept:]
	}
	s.takeShadow(a)
	a.msg(fmt.Sprintf("Joined %s: %dx%d, %s", s.addr, doc.cols, doc.rows, a.collabWho(s)))
}

// collabFont shows the page in the font a SAUCE font name means (the name kept when CharEdit lacks it).
func (a *App) collabFont(name string) {
	if name == "" {
		return
	}
	if f := fontBySauce(name, "pc"); f != nil {
		a.fontFor["pc"] = f.id
		a.ownFont = nil
		a.info.font, a.info.fontID = "", ""
		if !strings.EqualFold(f.sauce, name) {
			a.info.font, a.info.fontID = name, f.id
		}
	}
}

// sauceField is a SAUCE field from the network: Moebius's server sends the file's
// fields as they are in the record, padded with spaces to their length.
func sauceField(s string, n int) string { return strings.TrimRight(clean(s, n), " ") }

func collabNotes(s string) []string {
	s = clean(s, collabCommentRune)
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 255 {
		lines = lines[:255]
	}
	return lines
}

// collabApply makes a change another user made (any message that changes the
// page, or a cursor), on the page and in the shadow; true when it was taken.
func (a *App) collabApply(s *collabSession, u *collabUser, typ int, d cbIn) bool {
	t := s.tab
	switch typ {
	case cbDraw:
		if d.Block == nil || d.X < 0 || d.Y < 0 || d.X >= t.doc.cols || d.Y >= t.doc.rows {
			return false
		}
		v, ok := cbCell(*d.Block)
		if !ok {
			return false
		}
		a.collabPut(s, d.X, d.Y, v)
		if u != nil {
			u.y = d.Y // Moebius's "go to the user" uses it
		}
	case cbCursor, cbSelection, cbOperation:
		if u == nil || d.X < 0 || d.Y < 0 || d.X >= t.doc.cols || d.Y >= t.doc.rows {
			return false
		}
		u.x, u.y, u.cursor, u.sel = d.X, d.Y, true, typ != cbCursor
	case cbHideCursor:
		if u == nil {
			return false
		}
		u.cursor = false
	case cbStatus:
		if u == nil || d.Status < cbActive || d.Status > cbWeb {
			return false
		}
		u.status = d.Status
	case cbSauce:
		t.info.title, t.info.author, t.info.group = sauceField(d.Title, 35), sauceField(d.Author, 20), sauceField(d.Group, 20)
		t.info.notes = collabNotes(d.Comments)
		s.info = t.info
		t.dirty = true
	case cbIce:
		var v bool
		if json.Unmarshal(d.Value, &v) != nil {
			return false
		}
		t.ice, s.ice = v, v
		t.dirty = true
	case cbNinePx:
		var v bool
		if json.Unmarshal(d.Value, &v) != nil {
			return false
		}
		t.sauceFlags &^= 6
		if v {
			t.sauceFlags |= 4
		}
		s.ninePx = v
		t.dirty = true
	case cbFont:
		name := sauceField(d.FontName, 64)
		if name == "" {
			return false
		}
		a.withTab(t, func() string { a.collabFont(name); return "" })
		s.font = a.withTab(t, a.sauceFont)
		t.dirty = true
	case cbCanvasSize:
		if !collabSizeOK(d.Columns, d.Rows) {
			if !s.host {
				a.collabEnd(fmt.Sprintf("The page became %dx%d, too large to share: the drawing session ended", d.Columns, d.Rows))
			}
			return false
		}
		t.doc = cbResize(t.doc, d.Columns, d.Rows)
		s.shadow = cbResize(s.shadow, d.Columns, d.Rows)
		t.undo, t.redo = nil, nil // as Moebius: undo steps of the old size cannot come back
		t.cx, t.cy = min(t.cx, d.Columns-1), min(t.cy, d.Rows-1)
		t.sel, t.float = nil, nil
		t.dirty = true
	default:
		return false // set_bg (C64 pages), a block being placed, turned, flipped: nothing on the page yet
	}
	return true
}

// collabPut puts a cell that came from another user: on the page, in the shadow,
// and in the undo steps, so undo here takes back only what was drawn here.
func (a *App) collabPut(s *collabSession, x, y int, v uint32) {
	t := s.tab
	t.doc.set(x, y, v)
	s.shadow.set(x, y, v)
	for _, st := range [][]Snap{t.undo, t.redo} {
		for _, sn := range st {
			if sn.doc != nil && !sn.whole && sn.doc.cols == t.doc.cols && y < sn.doc.rows {
				sn.doc.set(x, y, v)
			}
		}
	}
	t.dirty = true
	s.gotCells++
}

// cbResize is a page cut or grown to cols x rows from its top left, as Moebius's resize_canvas.
func cbResize(d *Doc, cols, rows int) *Doc {
	n := d
	if d.cols != cols {
		n = d.withCols(cols)
	} else {
		n = d.clone()
	}
	if rows > n.rows {
		n.ensureRows(rows)
	} else {
		n.rows = rows
		n.cells = n.cells[:rows*cols]
	}
	return n
}

// note adds a line to the chat (id -1: a note of the session, as "Ann joined").
func (s *collabSession) note(id int, nick, text string) {
	if text == "" {
		return
	}
	s.chat = append(s.chat, collabLine{id: id, nick: nick, text: text, at: time.Now()})
	if len(s.chat) > collabChatKept {
		s.chat = s.chat[1:]
	}
	if id != s.id {
		s.unread++
	}
}

// ---------------------------------------------------------------- sending

// collabSendChanges sends what is different here from what the others have.
func (a *App) collabSendChanges(s *collabSession) {
	t := s.tab
	d, sh := t.doc, s.shadow
	if d.cols != sh.cols || d.rows != sh.rows {
		if !collabSizeOK(d.cols, d.rows) {
			a.collabEnd(fmt.Sprintf("The page is too large to share (%dx%d): the drawing session ended", d.cols, d.rows))
			return
		}
		s.out(cbCanvasSize, map[string]any{"columns": d.cols, "rows": d.rows}, true)
		oc, or := sh.cols, sh.rows
		s.shadow = cbResize(sh, d.cols, d.rows)
		sh = s.shadow
		for y := range sh.rows {
			for x := range sh.cols {
				if x >= oc || y >= or {
					sh.cells[y*sh.cols+x] = cbUnknown
				}
			}
		}
	}
	n := d.cols * d.rows
	for i, v := range d.cells[:n] {
		if !cbSame(v, sh.cells[i]) {
			sh.cells[i] = v
			s.out(cbDraw, map[string]any{"x": i % d.cols, "y": i / d.cols, "block": cbBlockOf(v)}, true)
			s.sentCells++
		}
	}
	if t.ice != s.ice {
		s.ice = t.ice
		s.out(cbIce, map[string]any{"value": s.ice}, true)
	}
	if t.ninePx() != s.ninePx {
		s.ninePx = t.ninePx()
		s.out(cbNinePx, map[string]any{"value": s.ninePx}, true)
	}
	if f := a.withTab(t, a.sauceFont); f != s.font && f != "" {
		s.font = f
		s.out(cbFont, map[string]any{"font_name": f}, true)
	}
	if i := t.info; i.title != s.info.title || i.author != s.info.author || i.group != s.info.group || strings.Join(i.notes, "\n") != strings.Join(s.info.notes, "\n") {
		s.info = i
		s.out(cbSauce, map[string]any{"title": i.title, "author": i.author, "group": i.group, "comments": strings.Join(i.notes, "\n")}, true)
	}
	// The text cursor, while the page is the one being worked on (Moebius shows it as a block).
	if a.Tab == t && a.dialog == nil {
		x, y := min(t.cx, d.cols-1), min(t.cy, d.rows-1)
		if x != s.cx || y != s.cy || !s.cursorShown {
			s.cx, s.cy, s.cursorShown = x, y, true
			s.out(cbCursor, map[string]any{"x": x, "y": y}, false)
		}
	} else if s.cursorShown {
		s.cursorShown = false
		s.out(cbHideCursor, map[string]any{}, false)
	}
}

// out sends one of our messages (our id is put in): to the server, or (hosting)
// to every guest; web viewers get only what changes the page (Moebius's send_all_including_guests).
func (s *collabSession) out(typ int, m map[string]any, page bool) {
	m["id"] = s.id
	if !s.host {
		if s.conn != nil {
			s.conn.send(typ, m)
		}
		return
	}
	s.broadcast(nil, typ, m, page)
}

// broadcast sends to every guest but from; page: web viewers too.
func (s *collabSession) broadcast(from *collabUser, typ int, data any, page bool) {
	for _, u := range s.users {
		if u.conn != nil && u != from && u.conn.in && (page || !u.web) {
			u.conn.send(typ, data)
		}
	}
}

// collabSay sends a chat line.
func (a *App) collabSay(text string) {
	s := collab
	text = clean(text, collabChatRunes)
	if s == nil || !s.ready || strings.TrimSpace(text) == "" {
		return
	}
	data := map[string]any{"id": s.id, "nick": s.nick, "group": s.group, "text": text}
	if s.host {
		data["time"] = time.Now().UnixMilli()
		s.keepHostChat(s.id, s.nick, s.group, text)
		s.broadcast(nil, cbChat, data, false)
	} else if s.conn != nil {
		s.conn.send(cbChat, data)
	}
	s.note(s.id, s.nick, text)
}

// ---------------------------------------------------------------- hosting

// keepHostChat keeps the chat history a guest gets on joining (Moebius's server keeps 32 lines).
func (s *collabSession) keepHostChat(id int, nick, group, text string) {
	s.hostChat = append(s.hostChat, cbChatJSON{ID: id, Nick: nick, Group: group, Text: text, Time: time.Now().UnixMilli()})
	if len(s.hostChat) > collabChatKept {
		s.hostChat = s.hostChat[1:]
	}
}

// collabGuestEvent handles a message from a guest while hosting: as Moebius's server (app/server.js Joint.message).
func (a *App) collabGuestEvent(s *collabSession, ev cbEvent) {
	c := ev.c
	if c == nil {
		return
	}
	u := c.user
	if ev.err != nil || ev.up {
		if u != nil && s.users[u.id] == u {
			delete(s.users, u.id)
			s.broadcast(u, cbLeave, map[string]int{"id": u.id}, false)
			if !u.web {
				s.note(-1, "", u.nick+" left")
			}
		}
		return
	}
	var d cbIn
	if json.Unmarshal(ev.msg.Data, &d) != nil {
		return
	}
	if u == nil { // not logged in: only CONNECTED is heard
		if ev.msg.Type != cbConnected || c.in {
			return
		}
		web := d.Nick == nil
		if (s.pass != "" && d.Pass != s.pass) || len(s.users) > collabMaxGuests {
			c.send(cbRefused, map[string]any{})
			c.closeAfter()
			return
		}
		u = &collabUser{id: s.nextID, web: web, group: clean(d.Group, collabNickRunes), conn: c}
		if !web {
			u.nick = clean(*d.Nick, collabNickRunes)
		} else {
			u.status = cbWeb
		}
		s.nextID++
		users := []cbUserJSON{} // never null: Moebius runs for...of over it, IcyDraw refuses a null list
		for _, o := range s.users {
			j := cbUserJSON{ID: o.id, Group: o.group, Status: o.status}
			if !o.web {
				nick := o.nick
				j.Nick = &nick
			}
			users = append(users, j)
		}
		doc := a.withTabDoc(s.tab)
		c.user, c.in = u, true
		c.login.Store(true)
		s.users[u.id] = u
		if web {
			c.send(cbConnected, map[string]any{"id": u.id, "doc": doc})
		} else {
			c.send(cbConnected, map[string]any{"id": u.id, "doc": doc, "users": users, "chat_history": append([]cbChatJSON{}, s.hostChat...), "status": cbActive})
			s.note(-1, "", u.nick+" joined")
		}
		j := map[string]any{"id": u.id, "group": u.group, "status": u.status}
		if !web {
			j["nick"] = u.nick
		}
		s.broadcast(u, cbJoin, j, false)
		return
	}
	switch ev.msg.Type {
	case cbConnected:
		return
	case cbChat:
		if u.web {
			return
		}
		if d.Nick != nil { // Moebius lets a user change the nick in its chat
			u.nick = clean(*d.Nick, collabNickRunes)
		}
		u.group = clean(d.Group, collabNickRunes)
		text := clean(d.Text, collabChatRunes)
		if text == "" {
			return
		}
		s.keepHostChat(u.id, u.nick, u.group, text)
		s.note(u.id, u.nick, text)
		s.broadcast(u, cbChat, map[string]any{"id": u.id, "nick": u.nick, "group": u.group, "text": text, "time": time.Now().UnixMilli()}, false)
		return
	case cbStatus:
		if a.collabApply(s, u, cbStatus, d) {
			s.broadcast(nil, cbStatus, map[string]int{"id": u.id, "status": u.status}, false)
		}
		return
	case cbPasteAsSelection:
		if b := d.Blocks; !u.web && b != nil && b.Columns > 0 && b.Rows > 0 && b.Columns <= collabMaxCols && b.Rows <= maxRowsFor(b.Columns) &&
			b.Columns*b.Rows <= collabMaxCells && len(b.Data) == b.Columns*b.Rows {
			for _, k := range b.Data {
				if _, ok := cbCell(k); !ok {
					return
				}
			}
			s.broadcast(u, cbPasteAsSelection, map[string]any{"id": u.id, "blocks": b}, false)
		}
		return
	case cbRotate, cbFlipX, cbFlipY:
		if !u.web {
			s.broadcast(u, ev.msg.Type, map[string]int{"id": u.id}, false)
		}
		return
	}
	if u.web || !a.collabApply(s, u, ev.msg.Type, d) {
		return // a viewer changes nothing; a message that does not fit is dropped
	}
	// Pass it on to the others, built anew from what was checked.
	t := s.tab
	var out any
	page := true
	switch ev.msg.Type {
	case cbDraw:
		out = map[string]any{"id": u.id, "x": d.X, "y": d.Y, "block": cbBlockOf(t.doc.get(d.X, d.Y))}
	case cbCursor, cbSelection, cbOperation:
		out, page = map[string]int{"id": u.id, "x": d.X, "y": d.Y}, false
	case cbHideCursor:
		out, page = map[string]int{"id": u.id}, false
	case cbSauce:
		out = map[string]any{"id": u.id, "title": t.info.title, "author": t.info.author, "group": t.info.group, "comments": strings.Join(t.info.notes, "\n")}
	case cbIce:
		out = map[string]any{"id": u.id, "value": s.ice}
	case cbNinePx:
		out = map[string]any{"id": u.id, "value": s.ninePx}
	case cbFont:
		out = map[string]any{"id": u.id, "font_name": clean(d.FontName, 64)}
	case cbCanvasSize:
		out = map[string]any{"id": u.id, "columns": d.Columns, "rows": d.Rows}
	default:
		return
	}
	s.broadcast(u, ev.msg.Type, out, page)
}

// withTabDoc is a tab's page as libtextmode.compress gives it, for a guest joining.
func (a *App) withTabDoc(t *Tab) cbDoc {
	old := a.Tab
	a.Tab = t
	defer func() { a.Tab = old; a.setPalette() }()
	a.setPalette()
	return a.cbDocOf()
}

// cbDocOf is the page being worked on in Moebius's form.
func (a *App) cbDocOf() cbDoc {
	d := a.doc
	n := d.cols * d.rows
	code, fg, bg := make([]int, n), make([]int, n), make([]int, n)
	for i, v := range d.cells[:n] {
		b := cbBlockOf(v)
		code[i], fg[i], bg[i] = b.Code, b.Fg, b.Bg
	}
	pal := make([]cbRGB, 16)
	bios := biosPalette(a.pal)
	for i, c := range bios { // Moebius keeps 6 bits
		pal[i] = cbRGB{int(c>>16&255) >> 2, int(c>>8&255) >> 2, int(c&255) >> 2}
	}
	notes := strings.Join(a.info.notes, "\n")
	font := a.sauceFont()
	if font == "" { // an own font: Moebius cannot load a font without a name
		font = "IBM VGA"
	}
	return cbDoc{Columns: d.cols, Rows: d.rows, Title: a.info.title, Author: a.info.author, Group: a.info.group,
		Date: time.Now().Format("20060102"), Palette: pal, FontName: font, Ice: a.ice, NinePx: a.ninePx(),
		Comments: notes, Compressed: &cbRuns{Code: cbRunsOf(code), Fg: cbRunsOf(fg), Bg: cbRunsOf(bg)}}
}

// collabWho names who else is in the session, in a few words.
func (a *App) collabWho(s *collabSession) string {
	var names []string
	web := 0
	for _, u := range s.collabOthers() {
		if u.web {
			web++
			continue
		}
		n := u.nick
		if n == "" {
			n = "Guest"
		}
		switch u.status {
		case cbIdle:
			n += " (idle)"
		case cbAway:
			n += " (away)"
		}
		names = append(names, n)
	}
	out := "nobody else yet"
	if len(names) > 0 {
		out = "with " + strings.Join(names, ", ")
	}
	if web > 0 {
		out += fmt.Sprintf(", %d watching", web)
	}
	return out
}

// collabOthers are the session's users but us, in the order they came.
func (s *collabSession) collabOthers() []*collabUser {
	var out []*collabUser
	for _, u := range s.users {
		if u.id != s.id {
			out = append(out, u)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].id < out[j-1].id; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// collabAddrs are this computer's addresses others may join at (hosting on all).
func collabAddrs() []string {
	var out []string
	ifs, _ := net.InterfaceAddrs()
	for _, ad := range ifs {
		if n, ok := ad.(*net.IPNet); ok && !n.IP.IsLoopback() && n.IP.To4() != nil && !n.IP.IsLinkLocalUnicast() {
			out = append(out, n.IP.String())
		}
	}
	return out
}
