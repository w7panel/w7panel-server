package types

import (
	"encoding/json"
	"testing"

	userv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/user/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestToArrayKeepsUserMenuAndRole(t *testing.T) {
	user := NewK3kUser(&userv1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "demo"},
		Spec: userv1alpha1.UserSpec{
			Role:      "normal",
			MenuRules: []string{"zpk"},
		},
	})

	if user.ToArray()[W7_ROLE] != "normal" {
		t.Fatal("user role was overridden")
	}
	var menuRules []string
	if err := json.Unmarshal([]byte(user.ToArray()[W7_MENU]), &menuRules); err != nil {
		t.Fatalf("unmarshal menu rules: %v", err)
	}
	if !containsMenu(menuRules, "zpk") {
		t.Fatalf("menu rules = %v, want zpk retained", menuRules)
	}
}

func containsMenu(menuRules []string, menu string) bool {
	for _, rule := range menuRules {
		if rule == menu {
			return true
		}
	}
	return false
}
