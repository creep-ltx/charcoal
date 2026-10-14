package main

import "testing"

// The arrow keys step over the lines between groups, both ways and round the ends,
// and a click finds the item under it, never a line.
func TestMenuPopup(t *testing.T) {
	p := &Popup{menu: true, hot: -1, items: []string{"Undo", "Redo", "", "Cut"}, r: Rect{10, 20, 200, 0}}
	p.r.h = p.height()
	var got []int
	for range 4 {
		p.step(1)
		got = append(got, p.hot)
	}
	if want := []int{0, 1, 3, 0}; !equalInts(got, want) {
		t.Errorf("down: %v, want %v", got, want)
	}
	p.hot = -1
	p.step(-1)
	if p.hot != 3 {
		t.Errorf("up from none: %d, want 3", p.hot)
	}
	for i := range p.items {
		r := p.row(i)
		want := i
		if p.items[i] == "" {
			want = -1
		}
		if got := p.at(r.x+5, r.y+r.h/2); got != want {
			t.Errorf("at row %d: %d", i, got)
		}
	}
	if last := p.row(3); last.y+last.h+2 != p.r.y+p.r.h {
		t.Errorf("the rows end at %d, the list at %d", last.y+last.h+2, p.r.y+p.r.h)
	}
}

// Every menu item runs something, and the menus show the keys in use.
func TestMenuBarItems(t *testing.T) {
	for _, m := range menuBar {
		for _, it := range m.items {
			if it.label != "" && it.action == "" && it.run == nil {
				t.Errorf("%s > %s does nothing", m.title, it.label)
			}
		}
	}
	if k := (menuItem{action: "animate"}).keys(); k != keyName("animate") {
		t.Errorf("Animate shows %q, its key is %q", k, keyName("animate"))
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
