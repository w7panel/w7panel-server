package agentpod

import "testing"

func TestRegisterAndLookup(t *testing.T) {
	if err := Register("2001:db8::10", "2001:db8::11"); err != nil {
		t.Fatal(err)
	}
	if got, ok := Lookup("2001:db8::10"); !ok || got != "2001:db8::11" {
		t.Fatalf("lookup = %q, %t", got, ok)
	}
	if err := Register("bad", "10.0.0.1"); err == nil {
		t.Fatal("expected invalid IP to fail")
	}
}
