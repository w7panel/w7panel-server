package k8s

import "testing"

func TestLicenseCRDSpecRoundTrip(t *testing.T) {
	want := LicenseCRDSpec{
		AppId:         "app",
		AppSecret:     "secret",
		FounderSaName: "admin",
		License:       "Y2VydA==",
	}

	obj := NewLicenseCRD(LicenseName, want)
	if obj.GetAPIVersion() != ConfigCRDGroup+"/"+ConfigCRDVersion {
		t.Fatalf("apiVersion = %q", obj.GetAPIVersion())
	}
	if obj.GetKind() != "License" {
		t.Fatalf("kind = %q", obj.GetKind())
	}
	if obj.GetName() != LicenseName {
		t.Fatalf("name = %q", obj.GetName())
	}

	got := ParseLicenseCRDSpec(obj)
	if got != want {
		t.Fatalf("ParseLicenseCRDSpec() = %#v, want %#v", got, want)
	}
}

func TestNewLicenseCRDAllowsEmptyLicense(t *testing.T) {
	obj := NewLicenseCRD(LicenseName, LicenseCRDSpec{
		AppId:         "app",
		AppSecret:     "secret",
		FounderSaName: "admin",
	})

	got := ParseLicenseCRDSpec(obj)
	if got.License != "" {
		t.Fatalf("license = %q, want empty", got.License)
	}
}

func TestOverSellingConfigCRDSpecRoundTrip(t *testing.T) {
	want := OverSellingConfigCRDSpec{
		CPU:          1000,
		Memory:       900,
		Storage:      800,
		BandWidth:    700,
		BandWidthNum: 600,
	}

	obj := NewOverSellingConfigCRD(OverSellingConfigName, want)
	if obj.GetAPIVersion() != ConfigCRDGroup+"/"+ConfigCRDVersion {
		t.Fatalf("apiVersion = %q", obj.GetAPIVersion())
	}
	if obj.GetKind() != "OverSellingConfig" {
		t.Fatalf("kind = %q", obj.GetKind())
	}
	if obj.GetName() != OverSellingConfigName {
		t.Fatalf("name = %q", obj.GetName())
	}

	got := ParseOverSellingConfigCRDSpec(obj)
	if got != want {
		t.Fatalf("ParseOverSellingConfigCRDSpec() = %#v, want %#v", got, want)
	}
}
