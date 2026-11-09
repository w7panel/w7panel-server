package middleware

import (
	"encoding/json"
	"testing"
)

func TestBuildMainMicroAppSiteConfigUsesSyncedDisplayData(t *testing.T) {
	data := map[string]json.RawMessage{
		"siteSetting": json.RawMessage(`{"spec":{"login":{"registrationEnabled":true,"indexPage":"resource"},"general":{"siteName":"主站","siteDescription":"主站描述","siteLogo":{"name":"logo","key":"image.png"},"filing":{"icp":"主站备案"}}}}`),
		"configMaps":  json.RawMessage(`{"logo":{"binaryData":{"image.png":"AQID"},"logoImageType":"data:image/png;base64,"}}`),
	}
	site, err := buildMainMicroAppSiteConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if site["siteName"] != "主站" || site["siteDescription"] != "主站描述" || site["logo"] != "data:image/png;base64,AQID" {
		t.Fatalf("main site display fields missing: %#v", site)
	}
	if site["filing"].(map[string]string)["icpnumber"] != "主站备案" || site["login"].(map[string]any)["registrationEnabled"] != true {
		t.Fatalf("main filing or login fields missing: %#v", site)
	}
}
