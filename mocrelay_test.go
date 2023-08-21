package mocrelay

import "testing"

func TestMocrelay(t *testing.T) {
	want := "Mocrelay (｀･ω･´)"
	if got := Mocrelay(); got != want {
		t.Errorf("got Mocrelay() == %q, want %q", got, want)
	}
}
