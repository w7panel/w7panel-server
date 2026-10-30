package permission

import (
	"testing"

	configv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/config/v1alpha1"
)

func TestExclusivePolicyParentWildcard(t *testing.T) {
	policy, err := ParseExclusivePolicy(`{"rules":[{"menuRule":"cluster/secret","menuKeys":["cluster-secret"],"clusterTypes":["main"],"apiRules":[{"path":"/panel-api/v1/secret","method":["get"]}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, menu := range []string{"*", "cluster/*", "cluster/secret", "cluster-secret"} {
		if policy.AllowsMenu(ChildClusterType, menu) {
			t.Errorf("child must not receive %q", menu)
		}
		if got := policy.FilterMenus(ChildClusterType, []string{menu}); len(got) != 0 {
			t.Errorf("child menu %q leaked through FilterMenus: %v", menu, got)
		}
		if err := policy.ValidateGrant(ChildClusterType, []string{menu}, nil); err == nil {
			t.Errorf("child grant %q accepted", menu)
		}
		if !policy.AllowsMenu(MainClusterType, menu) {
			t.Errorf("main must receive %q", menu)
		}
	}
	if !policy.AllowsMenu(ChildClusterType, "cluster/public") {
		t.Fatal("unrestricted child rejected")
	}
}

func TestExclusivePolicyMalformedConfig(t *testing.T) {
	for _, raw := range []string{"", `{`, `{}`, `{"rules":null}`, `{"rules":[{"menuRule":"cluster/private","clusterTypes":[]}]}`, `{"rules":[{"menuRule":"cluster/private","clusterTypes":["unknown"]}]}`, `{"rules":[{"menuRule":"cluster/private","exclusiveGroups":["ops"]}]}`, `{"rules":[],"unexpected":true}`} {
		if _, err := ParseExclusivePolicy(raw); err == nil {
			t.Errorf("malformed policy accepted: %q", raw)
		}
	}
	if policy, err := ParseExclusivePolicy(`{"rules":[]}`); err != nil || len(policy.Rules) != 0 {
		t.Fatalf("empty policy rejected: %v, %v", policy, err)
	}
}

func TestExclusivePolicyDuplicateAPIMethod(t *testing.T) {
	for _, raw := range []string{
		`{"rules":[{"menuRule":"a","clusterTypes":["main"],"apiRules":[{"path":"/api","method":["get","list"]}]}]}`,
		`{"rules":[{"menuRule":"a","clusterTypes":["main"],"apiRules":[{"path":"/api","method":["get"]}]},{"menuRule":"b","clusterTypes":["main"],"apiRules":[{"path":"/api","method":["list"]}]}]}`,
		`{"rules":[{"menuRule":"a","clusterTypes":["main"],"apiRules":[{"path":"/api/*","method":["get"]}]},{"menuRule":"b","clusterTypes":["main"],"apiRules":[{"path":"/api/private","method":["get"]}]}]}`,
		`{"rules":[{"menuRule":"a","clusterTypes":["main"],"apiRules":[{"path":"/api/*","method":["get"]}]},{"menuRule":"b","clusterTypes":["main"],"apiRules":[{"path":"/api/private","method":["create"]}]}]}`,
	} {
		if _, err := ParseExclusivePolicy(raw); err == nil {
			t.Errorf("overlapping API method accepted: %s", raw)
		}
	}
}

func TestExclusiveAPIPatterns(t *testing.T) {
	if _, err := ParseExclusivePolicy(`{"rules":[{"menuRule":"a","clusterTypes":["main"],"apiRules":[{"path":"/api/private/*","method":["get","create"]}]}]}`); err != nil {
		t.Fatalf("different methods on one API path rejected: %v", err)
	}
	policy, err := ParseExclusivePolicy(`{"rules":[{"menuRule":"a","clusterTypes":["main"],"apiRules":[{"path":"/api/private/*","method":["get"]}]}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if policy.AllowsAPI(ChildClusterType, "GET", "/api/private/object") {
		t.Fatal("restricted API accepted")
	}
	if !policy.AllowsAPI(ChildClusterType, "GET", "/api/public/object") {
		t.Fatal("unrelated API rejected")
	}
	if err := policy.ValidateGrant(ChildClusterType, nil, []configv1alpha1.PermissionAPIRule{{Path: "/api/*", Method: []string{"get"}}}); err == nil {
		t.Fatal("overlapping wildcard grant accepted")
	}
	if err := policy.ValidateGrant(ChildClusterType, nil, []configv1alpha1.PermissionAPIRule{{Path: "/api/public/*", Method: []string{"get"}}}); err != nil {
		t.Fatalf("unrelated grant rejected: %v", err)
	}
}
