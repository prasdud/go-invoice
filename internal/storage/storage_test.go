package storage

import "testing"

func TestKey(t *testing.T) {
	if got, want := Key("01J8ZZZZZZZZZZZZZZZZZZZZZZ"), "invoices/01J8ZZZZZZZZZZZZZZZZZZZZZZ.pdf"; got != want {
		t.Fatalf("Key = %q, want %q", got, want)
	}
}
