package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// ---------------------------------------------------------------- pieces

func TestCollabRuns(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for range 200 {
		n := 1 + r.Intn(500)
		v := make([]int, n)
		for i := range v {
			v[i] = r.Intn(3)
		}
		runs := cbRunsOf(v)
		got, err := cbExpand(runs, n, 0, 2)
		if err != nil || fmt.Sprint(got) != fmt.Sprint(v) {
			t.Fatalf("runs do not come back: %v %v", err, runs)
		}
	}
	// Hostile runs are refused, never expanded past the page.
	for _, c := range []struct {
		runs [][2]int
		n    int
	}{
		{[][2]int{{1, -1}}, 4},
		{[][2]int{{1, 4}}, 4},
		{[][2]int{{1, 1 << 40}}, 4},
		{[][2]int{{1, 1}}, 4},
		{[][2]int{{16, 3}}, 4},
		{[][2]int{{-1, 3}}, 4},
		{nil, 4},
	} {
		if _, err := cbExpand(c.runs, c.n, 0, 15); err == nil {
			t.Errorf("runs %v for %d cells taken", c.runs, c.n)
		}
	}
}

func TestCollabCells(t *testing.T) {
	for code := 0; code < 256; code++ {
		for fg := 0; fg < 16; fg++ {
			for bg := 0; bg < 16; bg += 5 {
				b := cbBlock{code, fg, bg}
				v, ok := cbCell(b)
				if !ok || cbBlockOf(v) != b {
					t.Fatalf("block %v -> %08x -> %v", b, v, cbBlockOf(v))
				}
			}
		}
	}
	// BIOS colour 1 is blue: CharEdit's 4.
	if v, _ := cbCell(cbBlock{65, 1, 4}); cellFg(v) != 4 || cellBg(v) != 1 {
		t.Errorf("BIOS colours not turned: fg %d bg %d", cellFg(v), cellBg(v))
	}
	// Unicode codes (an editor that sends characters) become CP437.
	if v, _ := cbCell(cbBlock{0x2588, 7, 0}); cellCh(v) != 0xDB {
		t.Errorf("U+2588 became %02X", cellCh(v))
	}
	if v, _ := cbCell(cbBlock{0x4E00, 7, 0}); cellCh(v) != '?' {
		t.Errorf("a character CP437 lacks became %02X", cellCh(v))
	}
	for _, b := range []cbBlock{{-1, 7, 0}, {65, 16, 0}, {65, 7, -1}, {0x110000, 7, 0}} {
		if _, ok := cbCell(b); ok {
			t.Errorf("block %v taken", b)
		}
	}
}

func TestCollabDocLimits(t *testing.T) {
	ok := cbDoc{Columns: 3, Rows: 2, Compressed: &cbRuns{Code: [][2]int{{65, 5}}, Fg: [][2]int{{7, 5}}, Bg: [][2]int{{0, 5}}}}
	if d, err := ok.docOf(); err != nil || d.cols != 3 || d.rows != 2 || cellCh(d.get(2, 1)) != 65 {
		t.Fatalf("a small page: %v", err)
	}
	for _, c := range []cbDoc{
		{Columns: 0, Rows: 2},
		{Columns: 80, Rows: -1},
		{Columns: 100000, Rows: 100000},
		{Columns: 1001, Rows: 1},
		{Columns: 80, Rows: collabMaxCells},
		{Columns: 3, Rows: 2},                                               // no cells
		{Columns: 3, Rows: 2, Data: []cbBlock{{65, 7, 0}}},                  // too few
		{Columns: 1, Rows: 1, Data: []cbBlock{{65, 99, 0}}},                 // a colour that is none
		{Columns: 3, Rows: 2, Compressed: &cbRuns{Code: [][2]int{{65, 5}}}}, // no colours
	} {
		if _, err := c.docOf(); err == nil {
			t.Errorf("page %dx%d taken", c.Columns, c.Rows)
		}
	}
}

func TestCollabPalette(t *testing.T) {
	ega := make([]cbRGB, 16)
	for i, c := range cbEGA {
		ega[i] = cbRGB{int(c>>16&255) >> 2, int(c>>8&255) >> 2, int(c&255) >> 2}
	}
	if cbPalette(ega) != nil {
		t.Error("Moebius's own palette is not the standard one")
	}
	ega8 := make([]cbRGB, 16)
	for i, c := range cbEGA {
		ega8[i] = cbRGB{int(c >> 16 & 255), int(c >> 8 & 255), int(c & 255)}
	}
	if cbPalette(ega8) != nil {
		t.Error("IcyDraw's 8-bit standard palette is not the standard one")
	}
	odd := append([]cbRGB(nil), ega...)
	odd[1] = cbRGB{63, 0, 0} // BIOS 1 (blue) is red here
	p := cbPalette(odd)
	if p == nil || p[4] != 0xFFFF0000 {
		t.Errorf("own palette: %v", p)
	}
	if cbPalette(ega[:15]) != nil || cbPalette(append(odd[:15:15], cbRGB{300, 0, 0})) != nil {
		t.Error("a palette that makes no sense taken")
	}
}

func TestCollabURL(t *testing.T) {
	for in, want := range map[string]string{
		"example.org":            "ws://example.org:8000/",
		"example.org/art":        "ws://example.org:8000/art",
		"example.org:9000/art":   "ws://example.org:9000/art",
		" ws://10.0.0.1:1/x ":    "ws://10.0.0.1:1/x",
		"wss://art.example/room": "wss://art.example:8000/room",
	} {
		if got, err := collabURL(in); err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "ws://", "http://x y"} {
		if _, err := collabURL(in); err == nil {
			t.Errorf("%q taken", in)
		}
	}
}

func TestCollabFileName(t *testing.T) {
	for in, want := range map[string]string{
		"ws://h:8000/art":            "art.ans",
		"ws://h:8000/":               "h.ans",
		"ws://h:8000/a/b/server.ANS": "server.ANS",
		"ws://h:8000/x:y":            "x_y.ans",
	} {
		if got := collabFileName(in); got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
}

func TestCollabClean(t *testing.T) {
	if got := clean("a\x1b[2Jb\nc", 100); got != "a[2Jbc" {
		t.Errorf("clean: %q", got)
	}
	if got := clean("x\ny", collabCommentRune); got != "x\ny" {
		t.Errorf("comment lines: %q", got)
	}
	if got := clean(strings.Repeat("é", 50), 10); got != strings.Repeat("é", 10) {
		t.Errorf("cut: %q", got)
	}
}

// collabFDocOf reads a page as a server sends it on joining (for FuzzParsers); nil when it cannot be.
func collabFDocOf(b []byte) *Doc {
	var d cbDoc
	if json.Unmarshal(b, &d) != nil {
		return nil
	}
	doc, err := d.docOf()
	if err != nil {
		return nil
	}
	return doc
}

// FuzzCollab sends any message to a joined session and to a hosted one: nothing may
// crash or make the page larger than a shared page may be.
func FuzzCollab(f *testing.F) {
	for _, s := range []string{
		`{"type":9,"data":{"id":1,"x":1,"y":1,"block":{"code":66,"fg":7,"bg":0}}}`,
		`{"type":16,"data":{"id":1,"columns":5,"rows":9}}`,
		`{"type":16,"data":{"id":1,"columns":1000,"rows":3000}}`,
		`{"type":0,"data":{"nick":"x","pass":""}}`,
		`{"type":10,"data":{"id":1,"nick":"n","text":"hi"}}`,
		`{"type":12,"data":{"title":"t","comments":"a
b"}}`,
		`{"type":13,"data":{"value":true}}`,
		`{"type":15,"data":{"font_name":"IBM VGA50"}}`,
		`{"type":17,"data":{"blocks":{"columns":1,"rows":1,"data":[{"code":1,"fg":2,"bg":3}]}}}`,
		`{"type":4,"data":{"id":1,"x":3,"y":2}}`,
	} {
		f.Add([]byte(s), []byte(`{"type":2,"data":{"id":7,"nick":"z"}}`))
	}
	f.Fuzz(func(t *testing.T, m1, m2 []byte) {
		for _, host := range []bool{false, true} {
			a := collabFApp(t)
			s := &collabSession{host: host, tab: a.Tab, in: make(chan cbEvent, 16), users: map[int]*collabUser{}, ready: true, nextID: 1}
			c := &cbConn{out: make(chan []byte, 1<<16), done: make(chan struct{})}
			if !host {
				s.conn, s.id = c, 0
			}
			s.users[0] = &collabUser{id: 0, nick: "me"}
			s.takeShadow(a)
			collab = s
			for _, b := range [][]byte{m1, m2, m1} {
				var m cbMsg
				if json.Unmarshal(b, &m) != nil {
					continue
				}
				a.collabEvent(s, cbEvent{c: c, msg: m})
				if collab == nil {
					break
				}
				a.collabSendChanges(s)
				d := s.tab.doc
				if !collabSizeOK(d.cols, d.rows) || len(d.cells) < d.cols*d.rows || s.shadow.cols != d.cols || s.shadow.rows != d.rows {
					t.Fatalf("a page of %dx%d (shadow %dx%d)", d.cols, d.rows, s.shadow.cols, s.shadow.rows)
				}
			}
			collab = nil
		}
	})
}

// ---------------------------------------------------------------- a raw client

// collabFRaw is a Moebius client reduced to the protocol: it keeps the page it
// was sent and what it heard, as Moebius's Connection does.
type collabFRaw struct {
	t      *testing.T
	ws     *websocket.Conn
	msgs   chan cbMsg
	id     int
	cols   int
	rows   int
	cells  []cbBlock
	heard  []cbMsg
	closed chan struct{}
}

func collabFDial(t *testing.T, url, nick, pass string) *collabFRaw {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ws, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	ws.SetReadLimit(64 << 20)
	r := &collabFRaw{t: t, ws: ws, msgs: make(chan cbMsg, 1<<16), closed: make(chan struct{})}
	go func() {
		defer close(r.closed)
		for {
			_, b, err := ws.Read(context.Background())
			if err != nil {
				return
			}
			var m cbMsg
			if json.Unmarshal(b, &m) == nil {
				r.msgs <- m
			}
		}
	}()
	t.Cleanup(func() { ws.Close(websocket.StatusNormalClosure, "") })
	data := map[string]any{"pass": pass, "group": "raw"}
	if nick != "" {
		data["nick"] = nick
	}
	r.send(cbConnected, data)
	return r
}

func (r *collabFRaw) send(typ int, data any) {
	b, _ := json.Marshal(map[string]any{"type": typ, "data": data})
	r.sendRaw(b)
	if typ == cbDraw || typ == cbCanvasSize { // as Moebius's client: its own changes are on its page at once
		var m cbMsg
		json.Unmarshal(b, &m)
		var d cbIn
		if json.Unmarshal(m.Data, &d) == nil && d.X >= 0 && d.Y >= 0 && (typ == cbCanvasSize || d.Block != nil && d.Block.Fg < 16) {
			r.take(m, d)
		}
	}
}

func (r *collabFRaw) sendRaw(b []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.ws.Write(ctx, websocket.MessageText, b); err != nil {
		r.t.Fatal(err)
	}
}

// until takes messages (keeping the page up to date) until want says yes;
// the editor's session is pumped meanwhile.
func (r *collabFRaw) until(what string, want func(cbMsg, cbIn) bool) cbIn {
	r.t.Helper()
	end := time.Now().Add(10 * time.Second)
	for time.Now().Before(end) {
		a.collabTick()
		select {
		case m := <-r.msgs:
			var d cbIn
			json.Unmarshal(m.Data, &d)
			r.heard = append(r.heard, m)
			r.take(m, d)
			if want(m, d) {
				return d
			}
		case <-time.After(3 * time.Millisecond):
		}
	}
	why := ""
	if collab != nil && collab.tab != nil {
		why = r.same(collab.tab)
	}
	r.t.Fatalf("waited in vain for %s (%s)", what, why)
	return cbIn{}
}

// settle takes messages until done says yes (looked at between messages too).
func (r *collabFRaw) settle(what string, done func() bool) {
	r.t.Helper()
	end := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(end) {
			r.t.Fatalf("waited in vain for %s (%s)", what, r.same(collab.tab))
		}
		a.collabTick()
		select {
		case m := <-r.msgs:
			var d cbIn
			json.Unmarshal(m.Data, &d)
			r.heard = append(r.heard, m)
			r.take(m, d)
		case <-time.After(3 * time.Millisecond):
		}
	}
}

func (r *collabFRaw) take(m cbMsg, d cbIn) {
	switch m.Type {
	case cbConnected:
		r.id = d.ID
		if d.Doc != nil {
			doc, err := d.Doc.docOf()
			if err != nil {
				r.t.Fatal(err)
			}
			r.cols, r.rows = doc.cols, doc.rows
			r.cells = make([]cbBlock, len(doc.cells[:doc.cols*doc.rows]))
			for i, v := range doc.cells[:doc.cols*doc.rows] {
				r.cells[i] = cbBlockOf(v)
			}
		}
	case cbDraw:
		if d.Block != nil && d.X < r.cols && d.Y < r.rows {
			r.cells[d.Y*r.cols+d.X] = *d.Block
		}
	case cbCanvasSize: // Moebius's resize_canvas
		if !collabSizeOK(d.Columns, d.Rows) {
			return
		}
		n := make([]cbBlock, d.Columns*d.Rows)
		for i := range n {
			n[i] = cbBlock{32, 7, 0}
		}
		for y := range min(r.rows, d.Rows) {
			for x := range min(r.cols, d.Columns) {
				n[y*d.Columns+x] = r.cells[y*r.cols+x]
			}
		}
		r.cells, r.cols, r.rows = n, d.Columns, d.Rows
	}
}

// same: the raw client's page is the editor's tab's.
func (r *collabFRaw) same(t *Tab) string {
	if r.cols != t.doc.cols || r.rows != t.doc.rows {
		return fmt.Sprintf("size %dx%d, the editor's %dx%d", r.cols, r.rows, t.doc.cols, t.doc.rows)
	}
	for i, b := range r.cells {
		if v := t.doc.cells[i]; cbBlockOf(v) != b {
			return fmt.Sprintf("cell %d,%d: %v, the editor's %v", i%r.cols, i/r.cols, b, cbBlockOf(v))
		}
	}
	return ""
}

func collabFPump(t *testing.T, what string, done func() bool) {
	t.Helper()
	end := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(end) {
			t.Fatalf("waited in vain for %s", what)
		}
		a.collabTick()
		time.Sleep(3 * time.Millisecond)
	}
}

func collabFFreePort(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func collabFApp(t *testing.T) *App {
	a := testApp(t, "pc")
	t.Cleanup(func() { a.collabEnd("") })
	return a
}

// collabFRandomPage fills the page with random cells.
func collabFRandomPage(d *Doc, seed int64) {
	r := rand.New(rand.NewSource(seed))
	for i := range d.cells[:d.cols*d.rows] {
		d.cells[i] = makeCell(r.Intn(256), r.Intn(16), r.Intn(16), 0)
	}
}

// ---------------------------------------------------------------- hosting

func TestCollabHost(t *testing.T) {
	a := collabFApp(t)
	a.doc = newDoc(81, 30) // an odd width, as Moebius pages may have
	collabFRandomPage(a.doc, 2)
	a.info.title, a.info.author, a.info.group, a.info.notes = "Title", "Me", "Grp", []string{"one", "two"}
	a.ice = true
	port := collabFFreePort(t)
	if err := a.collabHost(port, false, "boss", "secret"); err != nil {
		t.Fatal(err)
	}
	host := a.Tab
	url := fmt.Sprintf("ws://127.0.0.1:%d/anything", port)

	// A wrong password is refused.
	bad := collabFDial(t, url, "mallory", "nope")
	bad.until("REFUSED", func(m cbMsg, _ cbIn) bool { return m.Type == cbRefused })

	// Two users come in; the page they get is the host's.
	r1 := collabFDial(t, url, "ann", "secret")
	d := r1.until("CONNECTED", func(m cbMsg, _ cbIn) bool { return m.Type == cbConnected })
	if why := r1.same(host); why != "" {
		t.Fatal("ann's page: " + why)
	}
	if d.Doc.Title != "Title" || d.Doc.Comments != "one\ntwo" || !d.Doc.Ice || len(d.Users) != 1 || *d.Users[0].Nick != "boss" {
		t.Errorf("ann got %+v, users %+v", d.Doc, d.Users)
	}
	if m := r1.heard[0]; !strings.Contains(string(m.Data), `"chat_history":[]`) { // null would stop Moebius's chat.welcome
		t.Errorf("CONNECTED without an empty chat history: %.200s", m.Data)
	}
	r2 := collabFDial(t, url, "bob", "secret")
	r2.until("CONNECTED", func(m cbMsg, _ cbIn) bool { return m.Type == cbConnected })
	r1.until("bob's JOIN", func(m cbMsg, d cbIn) bool { return m.Type == cbJoin && d.Nick != nil && *d.Nick == "bob" })

	// Ann draws (giving a false id): the host's page and Bob's get it, from Ann's id.
	r1.send(cbDraw, map[string]any{"id": 0, "x": 80, "y": 29, "block": cbBlock{0xDB, 4, 1}})
	got := r2.until("ann's DRAW", func(m cbMsg, d cbIn) bool { return m.Type == cbDraw })
	if got.ID != r1.id || got.X != 80 || got.Y != 29 {
		t.Errorf("bob heard %+v", got)
	}
	if v := host.doc.get(80, 29); cellCh(v) != 0xDB || cellFg(v) != 1 || cellBg(v) != 4 {
		t.Errorf("the host's cell is %08x", v)
	}
	// What does not fit is dropped: outside the page, a colour that is none, a huge
	// chat line is cut, garbage is ignored.
	r1.send(cbDraw, map[string]any{"x": 81, "y": 0, "block": cbBlock{65, 7, 0}})
	r1.send(cbDraw, map[string]any{"x": -1, "y": 0, "block": cbBlock{65, 7, 0}})
	r1.send(cbDraw, map[string]any{"x": 0, "y": 0, "block": cbBlock{65, 99, 0}})
	r1.send(cbDraw, map[string]any{"x": "0", "y": 0, "block": cbBlock{65, 7, 0}})
	r1.sendRaw([]byte("{not json"))
	r1.send(cbCanvasSize, map[string]any{"columns": 100000, "rows": 100000})
	r1.send(cbChat, map[string]any{"nick": "ann", "text": strings.Repeat("x", 5000) + "\x1b[2J"})
	c := r2.until("ann's CHAT", func(m cbMsg, d cbIn) bool { return m.Type == cbChat })
	if len([]rune(c.Text)) != collabChatRunes || strings.Contains(c.Text, "\x1b") {
		t.Errorf("chat came on as %d runes", len([]rune(c.Text)))
	}
	if host.doc.cols != 81 || host.doc.rows != 30 || cellCh(host.doc.get(0, 0)) == 65 && cellFg(host.doc.get(0, 0)) == 7 {
		t.Errorf("a message that does not fit changed the page")
	}
	if n := len(collab.chat); n == 0 || collab.chat[n-1].nick != "ann" {
		t.Errorf("the host's chat: %+v", collab.chat)
	}

	// The host draws, resizes, changes the SAUCE and iCE: both guests get it all.
	a.typeByte('H')
	host.doc.set(3, 4, makeCell(0xB1, 14, 1, 0))
	a.setCols(90)
	a.setRows(33)
	host.doc.set(89, 32, makeCell('Z', 12, 0, 0))
	a.info.title = "New title"
	a.ice = false
	a.collabSay("hello from the host")
	for _, r := range []*collabFRaw{r1, r2} {
		r.until("the host's changes", func(m cbMsg, d cbIn) bool { return m.Type == cbSauce })
		if why := r.same(host); why != "" {
			t.Errorf("after the host's changes: %s", why)
		}
	}
	// The cursor and chat came too.
	sawSauce, sawChat := false, false
	for _, m := range r1.heard {
		var d cbIn
		json.Unmarshal(m.Data, &d)
		sawSauce = sawSauce || m.Type == cbSauce && d.Title == "New title"
		sawChat = sawChat || m.Type == cbChat && d.Text == "hello from the host" && d.ID == 0
	}
	if !sawSauce || !sawChat {
		t.Errorf("SAUCE %v chat %v", sawSauce, sawChat)
	}

	// Ann's cursor shows; Bob resizes the page and the host follows; undo here
	// takes back only the host's own step.
	r1.send(cbCursor, map[string]any{"x": 5, "y": 6})
	collabFPump(t, "ann's cursor", func() bool { u := collab.users[r1.id]; return u != nil && u.cursor && u.x == 5 && u.y == 6 })
	a.pushUndo()
	host.doc.set(1, 1, makeCell('U', 7, 0, 0))
	r2.send(cbDraw, map[string]any{"x": 2, "y": 2, "block": cbBlock{'B', 7, 0}})
	collabFPump(t, "bob's cell", func() bool { return cellCh(host.doc.get(2, 2)) == 'B' })
	a.doUndo()
	if cellCh(host.doc.get(2, 2)) != 'B' || cellCh(host.doc.get(1, 1)) == 'U' {
		t.Errorf("undo took back Bob's cell or not the host's: %c %c", cellCh(host.doc.get(2, 2)), cellCh(host.doc.get(1, 1)))
	}
	r2.send(cbCanvasSize, map[string]any{"columns": 70, "rows": 20})
	collabFPump(t, "bob's resize", func() bool { return host.doc.cols == 70 && host.doc.rows == 20 })
	r1.until("bob's resize", func(m cbMsg, d cbIn) bool { return m.Type == cbCanvasSize })
	for _, r := range []*collabFRaw{r1, r2} {
		r.settle("every page the same", func() bool { return r.same(host) == "" })
	}

	// Bob leaves: Ann hears it, the host forgets him.
	r2.ws.Close(websocket.StatusNormalClosure, "")
	r1.until("bob's LEAVE", func(m cbMsg, d cbIn) bool { return m.Type == cbLeave && d.ID == r2.id })
	if collab.users[r2.id] != nil {
		t.Error("bob is still there")
	}
	// A viewer without a nick and without the password is refused (Moebius would let it watch).
	w := collabFDial(t, url, "", "")
	w.until("REFUSED", func(m cbMsg, _ cbIn) bool { return m.Type == cbRefused })

	// Ending the session closes the guests' connections and the port.
	a.collabEnd("")
	select {
	case <-r1.closed:
	case <-time.After(5 * time.Second):
		t.Error("ann's connection stayed open")
	}
	if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second); err == nil {
		c.Close()
		t.Error("the port is still open")
	}
}

func TestCollabHostOnlyWhatIsAsked(t *testing.T) {
	a := collabFApp(t)
	port := collabFFreePort(t)
	if err := a.collabHost(port, false, "x", ""); err != nil {
		t.Fatal(err)
	}
	// Only 127.0.0.1 listens: another address of this computer does not answer.
	for _, ip := range collabAddrs() {
		if c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), 300*time.Millisecond); err == nil {
			c.Close()
			t.Errorf("%s:%d answers while hosting for this computer only", ip, port)
		}
	}
	a.collabEnd("")
	// Pages that cannot be shared.
	a.setMode("amiga")
	if err := a.collabHost(port, false, "x", ""); err == nil {
		t.Error("an Amiga page was shared")
	}
	a.setMode("pc")
	a.newFrame()
	if err := a.collabHost(port, false, "x", ""); err == nil {
		t.Error("a page with frames was shared")
	}
}

func TestCollabHostWebViewer(t *testing.T) {
	a := collabFApp(t)
	port := collabFFreePort(t)
	if err := a.collabHost(port, false, "x", ""); err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("ws://127.0.0.1:%d/", port)
	w := collabFDial(t, url, "", "")
	w.until("CONNECTED", func(m cbMsg, _ cbIn) bool { return m.Type == cbConnected })
	// A viewer cannot draw, but sees the drawing.
	w.send(cbDraw, map[string]any{"x": 0, "y": 0, "block": cbBlock{'W', 7, 0}})
	a.doc.set(1, 0, makeCell('H', 7, 0, 0))
	w.until("the host's cell", func(m cbMsg, d cbIn) bool { return m.Type == cbDraw && d.X == 1 })
	if cellCh(a.doc.get(0, 0)) == 'W' {
		t.Error("a web viewer drew")
	}
}

// ---------------------------------------------------------------- joining

// collabFStartServer runs a server built from Moebius's app/server.js
// (Joint.message), for joining without the real one: the same answers, the same
// relaying; its address.
func collabFStartServer(t *testing.T, doc cbDoc, pass string) string {
	t.Helper()
	port := collabFFreePort(t)
	type user struct {
		ws   *websocket.Conn
		nick *string
		id   int
	}
	in := make(chan func(), 1024)
	var users []*user
	cells, err := doc.docOf()
	if err != nil {
		t.Fatal(err)
	}
	cols := doc.Columns
	send := func(u *user, typ int, data any) {
		b, _ := json.Marshal(map[string]any{"type": typ, "data": data})
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		u.ws.Write(ctx, websocket.MessageText, b)
	}
	srv := &collabFHTTP{}
	srv.start(t, port, func(ws *websocket.Conn) {
		u := &user{ws: ws}
		for {
			_, b, err := ws.Read(context.Background())
			if err != nil {
				in <- func() {
					for _, o := range users {
						if o != u && o.nick != nil {
							send(o, cbLeave, map[string]int{"id": u.id})
						}
					}
				}
				return
			}
			var m cbMsg
			json.Unmarshal(b, &m)
			var d cbIn
			json.Unmarshal(m.Data, &d)
			raw := m.Data
			in <- func() {
				switch m.Type {
				case cbConnected:
					if d.Nick == nil || pass == "" || d.Pass == pass {
						var list []cbUserJSON
						for _, o := range users {
							list = append(list, cbUserJSON{ID: o.id, Nick: o.nick})
						}
						u.id, u.nick = len(users), d.Nick
						users = append(users, u)
						doc := doc
						code, fg, bg := make([]int, len(cells.cells)), make([]int, len(cells.cells)), make([]int, len(cells.cells))
						for i, v := range cells.cells[:cells.cols*cells.rows] {
							b := cbBlockOf(v)
							code[i], fg[i], bg[i] = b.Code, b.Fg, b.Bg
						}
						doc.Columns, doc.Rows = cells.cols, cells.rows
						doc.Compressed = &cbRuns{cbRunsOf(code[:cells.cols*cells.rows]), cbRunsOf(fg[:cells.cols*cells.rows]), cbRunsOf(bg[:cells.cols*cells.rows])}
						send(u, cbConnected, map[string]any{"id": u.id, "doc": doc, "users": list, "chat_history": []any{}, "status": 0})
						for _, o := range users {
							if o != u && o.nick != nil {
								send(o, cbJoin, map[string]any{"id": u.id, "nick": d.Nick, "status": 0})
							}
						}
					} else {
						send(u, cbRefused, map[string]any{})
					}
				case cbDraw:
					if d.X < cells.cols && d.Y < cells.rows && d.Block != nil {
						v, _ := cbCell(*d.Block)
						cells.set(d.X, d.Y, v)
						for _, o := range users {
							if o != u {
								send(o, m.Type, json.RawMessage(raw))
							}
						}
					}
				case cbCanvasSize:
					cells = cbResize(cells, d.Columns, d.Rows)
					cols = d.Columns
					for _, o := range users {
						if o != u {
							send(o, m.Type, json.RawMessage(raw))
						}
					}
				default:
					for _, o := range users {
						if o != u && o.nick != nil {
							send(o, m.Type, json.RawMessage(raw))
						}
					}
				}
			}
		}
	})
	go func() {
		for f := range in {
			f()
		}
	}()
	_ = cols
	return fmt.Sprintf("127.0.0.1:%d/test", port)
}

func TestCollabJoin(t *testing.T) {
	a := collabFApp(t)
	page := newDoc(80, 40)
	collabFRandomPage(page, 3)
	pal := make([]cbRGB, 16)
	for i, c := range cbEGA {
		pal[i] = cbRGB{int(c>>16&255) >> 2, int(c>>8&255) >> 2, int(c&255) >> 2}
	}
	pal[0] = cbRGB{10, 0, 20}
	doc := cbDoc{Title: "Shared", Author: "Ann", Group: "G", Palette: pal, FontName: "IBM VGA50", Ice: true, NinePx: true, Comments: "a\nb"}
	code, fg, bg := []int{}, []int{}, []int{}
	for _, v := range page.cells[:80*40] {
		b := cbBlockOf(v)
		code, fg, bg = append(code, b.Code), append(fg, b.Fg), append(bg, b.Bg)
	}
	doc.Columns, doc.Rows = 80, 40
	doc.Compressed = &cbRuns{cbRunsOf(code), cbRunsOf(fg), cbRunsOf(bg)}
	addr := collabFStartServer(t, doc, "pw")
	collabFJoinScenario(t, a, addr, "pw", func(tab *Tab) {
		for i, v := range page.cells[:80*40] {
			if !cbSame(tab.doc.cells[i], v) {
				t.Fatalf("cell %d differs", i)
			}
		}
		if !tab.ice || !tab.ninePx() || tab.info.title != "Shared" || strings.Join(tab.info.notes, "|") != "a|b" {
			t.Errorf("meta: ice %v 9px %v %+v", tab.ice, tab.ninePx(), tab.info)
		}
		if a.font().sauce != "IBM VGA50" {
			t.Errorf("font %q", a.font().sauce)
		}
		if a.pal[0] != 0xFF280051 {
			t.Errorf("palette 0: %08x", a.pal[0])
		}
	})
}

// collabFJoinScenario: the editor joins the server at addr, a raw client joins too,
// and both draw, resize and chat; the pages must stay the same.
func collabFJoinScenario(t *testing.T, a *App, addr, pass string, check func(*Tab)) {
	t.Helper()
	if err := a.collabJoin(addr, "charcoal", "grp", pass); err != nil {
		t.Fatal(err)
	}
	collabFPump(t, "the page", func() bool { return collab != nil && collab.ready })
	tab := collab.tab
	if check != nil {
		check(tab)
	}
	url, _ := collabURL(addr)
	r := collabFDial(t, url, "raw", pass)
	r.until("CONNECTED", func(m cbMsg, _ cbIn) bool { return m.Type == cbConnected })
	if why := r.same(tab); why != "" {
		t.Fatal("the raw client's page: " + why)
	}
	collabFPump(t, "raw's JOIN", func() bool {
		for _, u := range collab.users {
			if u.nick == "raw" {
				return true
			}
		}
		return false
	})
	// The editor draws: typing, a cell set by a tool, a fill (on a narrow page some
	// of it falls past the right edge, below the last row it adds rows).
	a.cx, a.cy = 0, 3
	a.typeByte('C')
	a.typeByte('E')
	tab.doc.set(10, 10, makeCell(0xDB, 9, 4, 0))
	for x := 0; x < 20; x++ {
		tab.doc.set(x, 15, makeCell(0xB0, 3, 5, 0))
	}
	a.collabTick()
	r.settle("the editor's cells", func() bool { return r.same(tab) == "" })
	// The raw client draws; the editor's page gets it.
	rx := min(5, tab.doc.cols-1)
	r.send(cbDraw, map[string]any{"id": r.id, "x": rx, "y": 5, "block": cbBlock{'R', 14, 1}})
	collabFPump(t, "raw's cell", func() bool { return cellCh(tab.doc.get(rx, 5)) == 'R' })
	if v := tab.doc.get(rx, 5); cellFg(v) != 11 || cellBg(v) != 4 { // BIOS yellow on blue
		t.Errorf("raw's cell is %08x", v)
	}
	// The editor makes the page taller and narrower (Enter past the last row adds rows).
	a.setCols(64)
	a.setRows(tab.doc.rows + 3)
	tab.doc.set(63, tab.doc.rows-1, makeCell('L', 7, 0, 0))
	r.until("the new size", func(m cbMsg, d cbIn) bool { return m.Type == cbCanvasSize })
	r.settle("the cells after it", func() bool { return r.same(tab) == "" })
	// Chat both ways.
	r.send(cbChat, map[string]any{"id": r.id, "nick": "raw", "group": "", "text": "hi there"})
	collabFPump(t, "raw's chat", func() bool { n := len(collab.chat); return n > 0 && collab.chat[n-1].text == "hi there" })
	a.collabSay("hello raw")
	r.until("the editor's chat", func(m cbMsg, d cbIn) bool { return m.Type == cbChat && d.Text == "hello raw" })
	// The raw client's cursor shows on the page.
	r.send(cbCursor, map[string]any{"id": r.id, "x": 0, "y": 8})
	collabFPump(t, "raw's cursor", func() bool {
		for _, u := range collab.users {
			if u.nick == "raw" && u.cursor && u.x == 0 && u.y == 8 {
				return true
			}
		}
		return false
	})
	// A second raw client that joins now gets the page as it is.
	r2 := collabFDial(t, url, "late", pass)
	r2.until("CONNECTED", func(m cbMsg, _ cbIn) bool { return m.Type == cbConnected })
	if why := r2.same(tab); why != "" {
		t.Error("the late client's page: " + why)
	}
	// Leaving keeps the page as a page of its own.
	a.collabEnd("")
	if !tab.dirty || a.tabIndex(tab) < 0 {
		t.Error("the page went with the session")
	}
}

func TestCollabJoinRefused(t *testing.T) {
	a := collabFApp(t)
	doc := cbDoc{Columns: 2, Rows: 1, Data: []cbBlock{{65, 7, 0}, {66, 7, 0}}}
	addr := collabFStartServer(t, doc, "pw")
	if err := a.collabJoin(addr, "x", "", "wrong"); err != nil {
		t.Fatal(err)
	}
	collabFPump(t, "the refusal", func() bool { return collab == nil })
	if !strings.Contains(a.message, "password") {
		t.Errorf("message %q", a.message)
	}
	// Nobody there.
	if err := a.collabJoin(fmt.Sprintf("127.0.0.1:%d", collabFFreePort(t)), "x", "", ""); err != nil {
		t.Fatal(err)
	}
	collabFPump(t, "the failure", func() bool { return collab == nil })
	if !strings.Contains(a.message, "Could not join") {
		t.Errorf("message %q", a.message)
	}
}

// TestCollabHostileServer: a server that sends nonsense cannot hurt the editor.
func TestCollabHostileServer(t *testing.T) {
	for _, first := range []string{
		`{"type":0,"data":{"id":1,"doc":{"columns":1000000,"rows":1000000,"compressed_data":{"code":[[65,999999999999]],"fg":[],"bg":[]}}}}`,
		`{"type":0,"data":{"id":1,"doc":{"columns":2,"rows":2,"compressed_data":{"code":[[65,-5]],"fg":[[7,3]],"bg":[[0,3]]}}}}`,
		`{"type":0,"data":{"id":1}}`,
		`{"type":0,"data":"x"}`,
	} {
		a := collabFApp(t)
		port := collabFFreePort(t)
		srv := &collabFHTTP{}
		srv.start(t, port, func(ws *websocket.Conn) {
			ws.Read(context.Background())
			ws.Write(context.Background(), websocket.MessageText, []byte(first))
			time.Sleep(time.Second)
		})
		a.collabJoin(fmt.Sprintf("127.0.0.1:%d", port), "x", "", "")
		collabFPump(t, "the end", func() bool { return collab == nil })
		if a.doc.cols*a.doc.rows > 80*25 {
			t.Errorf("a page of %dx%d from %s", a.doc.cols, a.doc.rows, first)
		}
	}
	// After joining: messages that do not fit are dropped, the page keeps its size limits.
	a := collabFApp(t)
	port := collabFFreePort(t)
	srv := &collabFHTTP{}
	srv.start(t, port, func(ws *websocket.Conn) {
		ws.Read(context.Background())
		w := func(s string) { ws.Write(context.Background(), websocket.MessageText, []byte(s)) }
		w(`{"type":0,"data":{"id":1,"doc":{"columns":2,"rows":2,"data":[{"code":65,"fg":7,"bg":0},{"code":65,"fg":7,"bg":0},{"code":65,"fg":7,"bg":0},{"code":65,"fg":7,"bg":0}]}}}`)
		w(`{"type":9,"data":{"id":5,"x":5,"y":0,"block":{"code":66,"fg":7,"bg":0}}}`)
		w(`{"type":9,"data":{"id":5,"x":0,"y":0,"block":{"code":66,"fg":70,"bg":0}}}`)
		w(`{"type":9,"data":{"id":5,"x":1e300,"y":0,"block":{"code":66,"fg":7,"bg":0}}}`)
		w(`{"type":2,"data":{"id":5,"nick":"` + strings.Repeat("N", 10000) + `"}}`)
		w(`{"type":10,"data":{"id":5,"text":"` + strings.Repeat("T", 100000) + `"}}`)
		w(`{"type":15,"data":{"id":5,"font_name":"` + strings.Repeat("F", 1000) + `"}}`)
		w(`{"type":12,"data":{"id":5,"title":"` + strings.Repeat("S", 1000) + `"}}`)
		w(`{"type":9,"data":{"id":5,"x":1,"y":1,"block":{"code":67,"fg":7,"bg":0}}}`)
		time.Sleep(3 * time.Second)
	})
	a.collabJoin(fmt.Sprintf("127.0.0.1:%d", port), "x", "", "")
	collabFPump(t, "the last cell", func() bool { return collab != nil && collab.ready && cellCh(collab.tab.doc.get(1, 1)) == 67 })
	tab := collab.tab
	if tab.doc.cols != 2 || cellCh(tab.doc.get(0, 0)) != 65 || len([]rune(tab.info.title)) > 35 {
		t.Errorf("the page took what did not fit: %dx%d %q", tab.doc.cols, tab.doc.rows, tab.info.title)
	}
	for _, u := range collab.users {
		if len([]rune(u.nick)) > collabNickRunes {
			t.Errorf("a nick of %d runes", len([]rune(u.nick)))
		}
	}
	for _, l := range collab.chat {
		if len([]rune(l.text)) > collabChatRunes {
			t.Errorf("a chat line of %d runes", len([]rune(l.text)))
		}
	}
}

// ---------------------------------------------------------------- the real servers

// TestCollabMoebiusServer joins Moebius's own server (app/server.js run with node by
// tools/collab/moebius-server.js; CHARCOAL_COLLAB_MOEBIUS = a folder holding Moebius's
// app/ as moebius-app/ and node_modules/: see tools/collab/README.md), serving each Moebius test file:
// the page joined must be the one Moebius's reader made of it.
func TestCollabMoebiusServer(t *testing.T) {
	dir := os.Getenv("CHARCOAL_COLLAB_MOEBIUS")
	if dir == "" {
		t.Skip("CHARCOAL_COLLAB_MOEBIUS not set")
	}
	n := 0
	for _, name := range moebiusFiles(t) {
		r := readMoebiusRef(t, name)
		if r.Utf8 || r.Nosauce || r.FontBytes != "" || strings.HasPrefix(r.Font, "Amiga") || strings.HasSuffix(name, ".asc") {
			continue // UTF-8 / no SAUCE: the reader's guesses; own fonts and Amiga pages do not travel
		}
		t.Run(name, func(t *testing.T) {
			a := collabFApp(t)
			addr := collabFMoebius(t, dir, filepath.Join("testdata/moebius", name), "pw")
			collabFJoinScenario(t, a, addr, "pw", func(tab *Tab) {
				old := a.Tab
				a.Tab = tab
				defer func() { a.Tab = old }()
				if got := strings.Join(tab.info.notes, "\n"); name == "sauce-utf8.ans" && got != r.Comments && strings.HasSuffix(r.Comments, got) {
					// Moebius's server reads UTF-8 comments by characters where the record
					// has bytes: the line loses its first letter (*verified*: its CONNECTED
					// says "ommentar på svenska"). CharEdit shows what it is sent.
					r.Comments = got
				}
				if bad, named := moebiusDiffs(tab.doc, r); bad > 0 {
					t.Errorf("%d cells differ: %v", bad, named)
				}
				checkMoebiusMeta(t, a, r, name)
			})
		})
		n++
	}
	if n < 10 {
		t.Errorf("only %d files", n)
	}
}

// TestCollabMoebiusHost: Moebius's own client code (libtextmode.uncompress and
// Connection.message's handling, run in node by tools/collab/moebius-client.js)
// joins CharEdit's session and reads the page.
func TestCollabMoebiusHost(t *testing.T) {
	dir := os.Getenv("CHARCOAL_COLLAB_MOEBIUS")
	if dir == "" {
		t.Skip("CHARCOAL_COLLAB_MOEBIUS not set")
	}
	out := filepath.Join(t.TempDir(), "doc.json")
	collabFHostedBy(t, "hello from Moebius", func(url string) *exec.Cmd {
		cmd := exec.Command("node", "tools/collab/moebius-client.js", filepath.Join(dir, "moebius-app"), url, "pw", out)
		cmd.Env = append(os.Environ(), "NODE_PATH="+filepath.Join(dir, "node_modules"))
		return cmd
	}, func([]byte) ([]byte, error) { return os.ReadFile(out) })
}

// TestCollabIcyDrawHost: IcyDraw's client (icy_engine_edit's connect, the example
// tools/collab/icydraw-collabcli.rs, CHARCOAL_COLLAB_ICYCLIENT) joins CharEdit's session.
func TestCollabIcyDrawHost(t *testing.T) {
	bin := os.Getenv("CHARCOAL_COLLAB_ICYCLIENT")
	if bin == "" {
		t.Skip("CHARCOAL_COLLAB_ICYCLIENT not set")
	}
	collabFHostedBy(t, "hello from IcyDraw", func(url string) *exec.Cmd {
		return exec.Command(bin, url, "pw")
	}, func(b []byte) ([]byte, error) {
		if i := strings.Index(string(b), `{"columns"`); i >= 0 {
			return b[i:], nil
		}
		return nil, fmt.Errorf("no page in its output")
	})
}

// collabFHostedBy hosts a page, lets another editor's client join (it draws its
// last cell, says hello, waits for the host's "M" at 0,0) and compares the page it
// ends with.
func collabFHostedBy(t *testing.T, hello string, client func(url string) *exec.Cmd, page func(out []byte) ([]byte, error)) {
	a := collabFApp(t)
	a.doc = newDoc(83, 27)
	collabFRandomPage(a.doc, 9)
	a.ice = true
	a.info.title, a.info.notes = "Hosted", []string{"line one", "line two"}
	port := collabFFreePort(t)
	if err := a.collabHost(port, false, "host", "pw"); err != nil {
		t.Fatal(err)
	}
	host := a.Tab
	cmd := client(fmt.Sprintf("ws://127.0.0.1:%d/", port))
	done := make(chan []byte)
	go func() {
		b, err := cmd.CombinedOutput()
		if err != nil {
			b = append(b, []byte(err.Error())...)
		}
		done <- b
	}()
	collabFPump(t, "the client's cell", func() bool {
		select {
		case b := <-done:
			t.Fatalf("the client ended:\n%s", b)
		default:
		}
		return cellCh(host.doc.get(82, 26)) == 0xDB
	})
	if v := host.doc.get(82, 26); cellFg(v) != 1 || cellBg(v) != 4 { // BIOS red on blue
		t.Errorf("the client's cell is %08x", v)
	}
	host.doc.set(0, 0, makeCell('M', 14, 1, 0))
	var log []byte
	for log == nil {
		a.collabTick()
		select {
		case log = <-done:
		case <-time.After(3 * time.Millisecond):
		}
	}
	collabFPump(t, "the client's chat", func() bool {
		for _, l := range collab.chat {
			if l.text == hello {
				return true
			}
		}
		return false
	})
	b, err := page(log)
	if err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	var got struct {
		Columns, Rows int
		Data          []cbBlock
		Title         string
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("%v\n%s", err, log)
	}
	if got.Columns != 83 || got.Rows != 27 || len(got.Data) != 83*27 || got.Title != "Hosted" {
		t.Fatalf("the client read %dx%d %q\n%s", got.Columns, got.Rows, got.Title, log)
	}
	for i, b := range got.Data {
		if cbBlockOf(host.doc.cells[i]) != b {
			t.Fatalf("cell %d,%d: the client has %v, the host %v", i%83, i/83, b, cbBlockOf(host.doc.cells[i]))
		}
	}
}

// TestCollabIcyDrawServer joins IcyDraw's server (icy_engine_edit's run_server,
// CHARCOAL_COLLAB_ICY = a program taking port, password, columns, rows and a
// backup folder: tools/collab/README.md).
func TestCollabIcyDrawServer(t *testing.T) {
	bin := os.Getenv("CHARCOAL_COLLAB_ICY")
	if bin == "" {
		t.Skip("CHARCOAL_COLLAB_ICY not set")
	}
	a := collabFApp(t)
	port := collabFFreePort(t)
	cmd := exec.Command(bin, strconv.Itoa(port), "pw", "80", "25", t.TempDir())
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	collabFWaitPort(t, port)
	collabFJoinScenario(t, a, fmt.Sprintf("127.0.0.1:%d/", port), "pw", func(tab *Tab) {
		if tab.doc.cols != 80 || tab.doc.rows != 25 {
			t.Errorf("IcyDraw's page is %dx%d", tab.doc.cols, tab.doc.rows)
		}
	})
}

func collabFWaitPort(t *testing.T, port int) {
	t.Helper()
	end := time.Now().Add(10 * time.Second)
	for time.Now().Before(end) {
		if c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
			c.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the server did not start")
}

// collabFMoebius runs Moebius's server on a file; its address.
func collabFMoebius(t *testing.T, dir, file, pass string) string {
	t.Helper()
	port := collabFFreePort(t)
	// A copy in a folder of its own: Moebius's server writes the page back to its
	// file when it closes, and hourly backups into the folder it runs in.
	tmp := t.TempDir()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(tmp, filepath.Base(file))
	if err := os.WriteFile(copyPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	script, _ := filepath.Abs("tools/collab/moebius-server.js")
	cmd := exec.Command("node", script, filepath.Join(dir, "moebius-app"), strconv.Itoa(port), pass, copyPath, "test")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "NODE_PATH="+filepath.Join(dir, "node_modules"))
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	collabFWaitPort(t, port)
	return fmt.Sprintf("127.0.0.1:%d/test", port)
}

// ---------------------------------------------------------------- a websocket server for the fakes

type collabFHTTP struct{}

func (collabFHTTP) start(t *testing.T, port int, serve func(*websocket.Conn)) {
	t.Helper()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		ws.SetReadLimit(64 << 20)
		serve(ws)
		ws.Close(websocket.StatusNormalClosure, "")
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
}
