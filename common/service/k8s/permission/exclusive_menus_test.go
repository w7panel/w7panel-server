package permission

import "testing"

func TestExclusiveMenusValidate(t *testing.T) {
	valid := ExclusiveMenus{Rules: []ExclusiveMenuRule{
		{MenuRule: "system", ClusterTypes: []string{"main"}},
		{MenuRule: "system/audit", ClusterTypes: []string{"main"}},
	}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, rules := range [][]ExclusiveMenuRule{
		{{MenuRule: "system", ClusterTypes: []string{"main", "main"}}},
		{{MenuRule: "system/*", ClusterTypes: []string{"main"}}},
		{{MenuRule: "system", ClusterTypes: []string{"main"}}, {MenuRule: "system/audit", ClusterTypes: []string{"child"}}},
	} {
		if err := (ExclusiveMenus{Rules: rules}).Validate(); err == nil {
			t.Fatalf("accepted invalid rules: %+v", rules)
		}
	}
}
