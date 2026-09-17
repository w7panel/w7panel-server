package console

import (
	"testing"
	"time"
)

func TestCloudAccessTokenCacheTTL(t *testing.T) {
	if got := cloudAccessTokenCacheTTL(0); got != time.Hour {
		t.Fatalf("zero expiry TTL = %s, want %s", got, time.Hour)
	}
	if got := cloudAccessTokenCacheTTL(time.Now().Add(30 * time.Second).Unix()); got != 0 {
		t.Fatalf("near-expiry TTL = %s, want 0", got)
	}
	remaining := 10 * time.Minute
	ttl := cloudAccessTokenCacheTTL(time.Now().Add(remaining).Unix())
	if ttl < remaining-time.Minute-time.Second || ttl > remaining-time.Minute {
		t.Fatalf("TTL = %s, want about %s", ttl, remaining-time.Minute)
	}
}
