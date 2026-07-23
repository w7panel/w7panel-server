package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestParseLegacyLicenseCRDSpec(t *testing.T) {
	want := LicenseCRDSpec{
		AppId:         "app",
		AppSecret:     "secret",
		FounderSaName: "admin",
		License:       "Y2VydA==",
	}

	obj := legacyLicenseCRD(want)

	got := ParseLicenseCRDSpec(obj)
	if got != want {
		t.Fatalf("ParseLicenseCRDSpec() = %#v, want %#v", got, want)
	}
}

func TestParseLegacyLicenseCRDSpecAllowsEmptyLicense(t *testing.T) {
	obj := legacyLicenseCRD(LicenseCRDSpec{
		AppId:         "app",
		AppSecret:     "secret",
		FounderSaName: "admin",
	})

	got := ParseLicenseCRDSpec(obj)
	if got.License != "" {
		t.Fatalf("license = %q, want empty", got.License)
	}
}

func legacyLicenseCRD(spec LicenseCRDSpec) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ConfigCRDGroup + "/" + ConfigCRDVersion)
	obj.SetKind("License")
	obj.SetName(LicenseName)
	obj.Object["spec"] = map[string]interface{}{
		"appId":         spec.AppId,
		"appSecret":     spec.AppSecret,
		"founderSaName": spec.FounderSaName,
		"license":       spec.License,
	}
	return obj
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

func TestFilingConfigCRDSpecRoundTrip(t *testing.T) {
	want := FilingConfigCRDSpec{
		IcpNumber: "icp",
		Number:    "12345678901234",
		Location:  "公网安备 12345678901234 号",
		License:   "https://example.com/license",
		Tbol:      "tbol",
	}

	obj := NewFilingConfigCRD(FilingConfigName, want)
	if obj.GetKind() != "FilingConfig" {
		t.Fatalf("kind = %q", obj.GetKind())
	}
	if obj.GetName() != FilingConfigName {
		t.Fatalf("name = %q", obj.GetName())
	}

	got := ParseFilingConfigCRDSpec(obj)
	if got != want {
		t.Fatalf("ParseFilingConfigCRDSpec() = %#v, want %#v", got, want)
	}
}

func TestDomainParseConfigCRDSpecRoundTrip(t *testing.T) {
	want := DomainParseConfigCRDSpec{
		Type:  "A",
		IPs:   []string{"1.1.1.1", "2.2.2.2"},
		Cname: "example.com",
	}

	obj := NewDomainParseConfigCRD(DomainParseConfigName, want)
	if obj.GetKind() != "DomainParseConfig" {
		t.Fatalf("kind = %q", obj.GetKind())
	}
	if obj.GetName() != DomainParseConfigName {
		t.Fatalf("name = %q", obj.GetName())
	}

	got := ParseDomainParseConfigCRDSpec(obj)
	if got.Type != want.Type || got.Cname != want.Cname || len(got.IPs) != len(want.IPs) {
		t.Fatalf("ParseDomainParseConfigCRDSpec() = %#v, want %#v", got, want)
	}
	for i := range want.IPs {
		if got.IPs[i] != want.IPs[i] {
			t.Fatalf("ParseDomainParseConfigCRDSpec().IPs[%d] = %q, want %q", i, got.IPs[i], want.IPs[i])
		}
	}
}

func TestContactConfigCRDSpecRoundTrip(t *testing.T) {
	want := ContactConfigCRDSpec{
		Type:     "qrcode",
		Link:     "https://example.com",
		Text:     "hello",
		Name:     "contact",
		ShowName: true,
		SelIcon:  "icon-customer-service",
		Icon:     "data:image/png;base64,aWNvbg==",
		Qrcode:   "data:image/png;base64,cXJjb2Rl",
		Style:    "1",
		Index:    2,
	}

	obj := NewContactConfigCRD("contact-us-test", want)
	if obj.GetKind() != "ContactConfig" {
		t.Fatalf("kind = %q", obj.GetKind())
	}

	got := ParseContactConfigCRDSpec(obj)
	if got != want {
		t.Fatalf("ParseContactConfigCRDSpec() = %#v, want %#v", got, want)
	}
}
