package main

import "testing"

func TestPrivateCalibrationDigest(t *testing.T) {
	mode := "cursor-neutral-v1"
	first, e := digest([]byte(`{"items":[{"id":1}],"next_cursor":"native"}`), mode)
	if e != nil {
		t.Fatal(e)
	}
	other, e := digest([]byte(`{"next_cursor":"parade","items":[{"id":1}]}`), mode)
	if e != nil || other != first {
		t.Fatal("opaque cursor or object key order changed content digest", e)
	}
	for _, raw := range []string{`{"items":[{"id":2}],"next_cursor":"native"}`, `{"items":[{"id":1}],"next_cursor":""}`} {
		changed, e := digest([]byte(raw), mode)
		if e != nil || changed == first {
			t.Fatal("record/lookahead change was hidden", e)
		}
	}
	for _, raw := range []string{`broken`, `[]`, `{"next_cursor":3}`} {
		if _, e := digest([]byte(raw), mode); e == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
	if _, e := digest([]byte("{\"id\":1}\n{\"id\":2}\n"), ""); e != nil {
		t.Fatal(e)
	}
	if _, e := digest([]byte("{\"id\":1}\nbroken\n"), ""); e == nil {
		t.Fatal("truncated export accepted")
	}
}
