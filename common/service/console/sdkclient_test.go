// nolint
package console

import (
	"os"
	"testing"
)

func disabledCreateProductOrder(t *testing.T) {
	os.Setenv("USER_AGENT", "we7test-beta")
	sdkClient, err := NewDefaultSdkClient()
	if err != nil {
		t.Fatal(err)
	}
	info, err := sdkClient.CreateDefaultProductOrder("10665")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(info)
}

func disabledCreatePanelSite(t *testing.T) {
	os.Setenv("USER_AGENT", "we7test-beta")
	os.Setenv("LOCAL_MOCK", "1")
	sdkClient, err := NewDefaultSdkClient()
	if err != nil {
		t.Fatal(err)
	}
	info, err := sdkClient.CreateSiteFromPanel("https://test.cc", "aaaa")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(info)
}

func disabledCreateProductOrder1(t *testing.T) {
	os.Setenv("USER_AGENT", "we7test-beta")
	sdkClient, err := NewDefaultSdkClient()
	if err != nil {
		t.Fatal(err)
	}
	info, err := sdkClient.CreateDefaultProductOrder("10665")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(info)
}

func disabledPrepareProduct(t *testing.T) {
	os.Setenv("USER_AGENT", "we7test-beta")
	sdkClient, err := NewDefaultSdkClient()
	if err != nil {
		t.Fatal(err)
	}
	info, err := sdkClient.PrepareProduct2()
	if err != nil {
		t.Fatal(err)
	}
	t.Log(info)
}

func disabledShowCoupon(t *testing.T) {
	os.Setenv("USER_AGENT", "we7test-beta")
	// os.Setenv("LOCAL_MOCK", "true")
	// os.Setenv("")
	sdkClient, err := NewDefaultSdkClient()
	if err != nil {
		t.Fatal(err)
	}
	info, err := sdkClient.GetCoupon("20251212184530-4H2P3ZVD")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(info)
}

func disabledUpdateCoupon(t *testing.T) {
	os.Setenv("USER_AGENT", "we7test-beta")
	// os.Setenv("LOCAL_MOCK", "true")
	// os.Setenv("")
	sdkClient, err := NewDefaultSdkClient()
	if err != nil {
		t.Fatal(err)
	}

	err = sdkClient.UpdateCoupon("20251229154927-Z2LTJTVR", "lock", "xxxddd")
	if err != nil {
		t.Fatal(err)
	}

}

// disabledOpenIDToPassportToken calls the fixed internal Console API.
func disabledOpenIDToPassportToken(t *testing.T) {
	// os.Setenv("USER_AGENT", "we7test-beta")
	// os.Setenv("LOCAL_MOCK", "1")
	sdkClient, err := NewDefaultSdkClient()
	if err != nil {
		t.Fatal(err)
	}
	info, err := sdkClient.OpenIdToCloudAccessToken("mzUZB6jb59EJKakQsGrVSg")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(info)
}

// disabledOpenIDToCode calls the fixed internal Console API.
func disabledOpenIDToCode(t *testing.T) {
	// os.Setenv("USER_AGENT", "we7test-beta")
	os.Setenv("LOCAL_MOCK", "1")
	sdkClient, err := NewDefaultSdkClient()
	if err != nil {
		t.Fatal(err)
	}
	info, err := sdkClient.OpenIdToCloudCode("mzUZB6jb59EJKakQsGrVSg", "510929")
	if err != nil {
		t.Fatal(err)
	}
	t.Log(info)
}
