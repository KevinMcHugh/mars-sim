package sim

import "testing"

func TestLogKindLabelsAreDistinct(t *testing.T) {
	seen := map[string]LogKind{}
	for k := LogNote; k < LogKindCount; k++ {
		label := k.String()
		if label == "" {
			t.Errorf("kind %d has an empty label", k)
		}
		if prev, ok := seen[label]; ok {
			t.Errorf("kinds %d and %d both use %q", prev, k, label)
		}
		seen[label] = k
	}
}

// logEventAt records its tile; logEvent records none.
func TestLogEventPlace(t *testing.T) {
	l := newEventLog(4)
	l.add(1, LogNote, "nowhere", Point{}, false)
	l.add(2, LogDeath, "here", Point{5, 6, 2}, true)
	got := l.tail(2)
	if got[0].HasAt {
		t.Errorf("a line without a place has one: %+v", got[0].At)
	}
	if !got[1].HasAt || got[1].At != (Point{5, 6, 2}) {
		t.Errorf("place = %+v (%v), want 5,6 on level 2", got[1].At, got[1].HasAt)
	}
}

// A project's site is the middle of its tasks on its first task's level.
func TestProjectSite(t *testing.T) {
	if _, ok := (&project{}).site(); ok {
		t.Error("a project with no tasks has a site")
	}
	p := &project{tasks: []*buildTask{
		{pos: Point{2, 2, 1}}, {pos: Point{6, 4, 1}}, {pos: Point{40, 40, 2}},
	}}
	if at, ok := p.site(); !ok || at != (Point{4, 3, 1}) {
		t.Errorf("site = %v %v, want 4,3 on level 1", at, ok)
	}
}
