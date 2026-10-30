package ckmsession

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/panelauth"
)

func TestExecutionAudiencesMarkOnlyEmbeddedToken(t *testing.T) {
	p := panelauth.Principal{Username: "alice", CVMName: "ckm-1", K3KNamespace: "k3k-alice", CKMUID: "uid-1"}
	bootstrap := Audiences(p)
	execution := executionAudiences(p)
	if bootstrap[2] != "" || execution[2] != "w7panel-ckm-embed:uid-1" || len(execution) != len(bootstrap) {
		t.Fatalf("unexpected audiences: bootstrap=%v execution=%v", bootstrap, execution)
	}
	for index := range bootstrap {
		if index != 2 && bootstrap[index] != execution[index] {
			t.Fatalf("audience %d changed: bootstrap=%v execution=%v", index, bootstrap, execution)
		}
	}
	if audiencesForBinding(p, "")[2] != "" || audiencesForBinding(p, "panel")[2] != "w7panel-ckm-embed:uid-1" {
		t.Fatal("historical CKM must keep its original execution audience")
	}
}
