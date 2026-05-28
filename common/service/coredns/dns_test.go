package coredns

import (
	"os"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	if os.Getenv("COREDNS_LIVE_TEST") != "true" {
		t.Skip("set COREDNS_LIVE_TEST=true to read live kube-system/coredns-custom")
	}
	json, err := ParseToJsonConfig()
	if err != nil {
		t.Error(err)
		return
	}
	jsonstr := string(json)
	t.Log(jsonstr)
}
