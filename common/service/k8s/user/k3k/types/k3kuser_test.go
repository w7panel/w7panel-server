package types

import (
	"encoding/json"
	"testing"

	userv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/user/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestToArrayCkmRequestRemovesZPKMenu(t *testing.T) {
	user := NewK3kUser(&userv1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "demo"},
		Spec: userv1alpha1.UserSpec{
			MenuRules: []string{"custom/menu", "zpk"},
		},
	})
	user.SetCkmName("demo-ckm")

	var menuRules []string
	if err := json.Unmarshal([]byte(user.ToArray()[W7_MENU]), &menuRules); err != nil {
		t.Fatalf("unmarshal menu rules: %v", err)
	}
	for _, menu := range menuRules {
		if menu == "zpk" {
			t.Fatal("CKM request user menu must not include zpk")
		}
	}
	if !containsMenu(menuRules, "custom/menu") {
		t.Fatalf("menu rules = %v, want custom menu retained", menuRules)
	}
}

func TestToArrayNonCkmRequestKeepsZPKMenu(t *testing.T) {
	user := NewK3kUser(&userv1alpha1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "demo"},
		Spec: userv1alpha1.UserSpec{
			MenuRules: []string{"zpk"},
		},
	})

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
