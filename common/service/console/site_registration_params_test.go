package console

import "testing"

func TestCreateSiteFromPanelParams(t *testing.T) {
	t.Run("site name and OpenID", func(t *testing.T) {
		params := createSiteFromPanelParams("https://site.example.com", "site-id", "示例站点", "openid")
		want := map[string]string{
			"url":            "https://site.example.com",
			"site_identifie": "site-id",
			"site_name":      "示例站点",
			"openid":         "openid",
		}
		for key, value := range want {
			if params[key] != value {
				t.Fatalf("params[%q] = %q, want %q", key, params[key], value)
			}
		}
	})

	t.Run("optional values omitted", func(t *testing.T) {
		params := createSiteFromPanelParams("https://site.example.com", "site-id", "", "")
		if _, exists := params["site_name"]; exists {
			t.Fatal("empty site_name must be omitted")
		}
		if _, exists := params["openid"]; exists {
			t.Fatal("empty openid must be omitted")
		}
	})
}
