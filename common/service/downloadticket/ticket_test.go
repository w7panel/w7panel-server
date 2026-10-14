package downloadticket

import "testing"

func TestTicketIsBoundAndLimited(t *testing.T) {
	token, _, err := Issue("upload/a.zip")
	if err != nil {
		t.Fatal(err)
	}
	if Consume(token, "upload/other.zip") {
		t.Fatal("ticket must be path-bound")
	}
	for i := 0; i < 4; i++ {
		if !Consume(token, "upload/a.zip") {
			t.Fatalf("use %d rejected", i+1)
		}
	}
	if Consume(token, "upload/a.zip") {
		t.Fatal("fifth use must be rejected")
	}
}
