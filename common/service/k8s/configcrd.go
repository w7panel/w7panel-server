package k8s

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	ConfigCRDGroup        = "w7panel.w7.com"
	ConfigCRDVersion      = "v1alpha1"
	K3kConfigName         = "config"
	K3sConfigName         = "config"
	LicenseName           = "license"
	OverSellingConfigName = "config"
	FilingConfigName      = "beian"
	DomainParseConfigName = "domain-parse"
)

var (
	K3kConfigGVR         = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "k3kconfigs"}
	K3sConfigGVR         = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "k3sconfigs"}
	LicenseGVR           = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "licenses"}
	OverSellingConfigGVR = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "oversellingconfigs"}
	FilingConfigGVR      = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "filingconfigs"}
	DomainParseConfigGVR = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "domainparseconfigs"}
	ContactConfigGVR     = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "contactconfigs"}
)

type LicenseCRDSpec struct {
	AppId         string
	AppSecret     string
	FounderSaName string
	License       string
}

type OverSellingConfigCRDSpec struct {
	CPU          int32
	Memory       int32
	Storage      int32
	BandWidth    int32
	BandWidthNum int32
}

type FilingConfigCRDSpec struct {
	IcpNumber string
	Number    string
	Location  string
	License   string
	Tbol      string
}

type DomainParseConfigCRDSpec struct {
	Type  string
	IPs   []string
	Cname string
}

type ContactConfigCRDSpec struct {
	Type     string
	Link     string
	Text     string
	Name     string
	ShowName bool
	SelIcon  string
	Icon     string
	Qrcode   string
	Style    string
	Index    int32
}

func ConfigCRDData(obj *unstructured.Unstructured) map[string]string {
	data, _, _ := unstructured.NestedStringMap(obj.Object, "spec", "data")
	if data == nil {
		data = map[string]string{}
	}
	return data
}

func NewConfigCRD(kind, name string, labels map[string]string, data map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ConfigCRDGroup + "/" + ConfigCRDVersion)
	obj.SetKind(kind)
	obj.SetName(name)
	obj.SetLabels(labels)
	obj.Object["spec"] = map[string]interface{}{
		"data": data,
	}
	return obj
}

func NewLicenseCRD(name string, spec LicenseCRDSpec) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ConfigCRDGroup + "/" + ConfigCRDVersion)
	obj.SetKind("License")
	obj.SetName(name)
	obj.Object["spec"] = map[string]interface{}{
		"appId":         spec.AppId,
		"appSecret":     spec.AppSecret,
		"founderSaName": spec.FounderSaName,
		"license":       spec.License,
	}
	return obj
}

func ParseLicenseCRDSpec(obj *unstructured.Unstructured) LicenseCRDSpec {
	appId, _, _ := unstructured.NestedString(obj.Object, "spec", "appId")
	appSecret, _, _ := unstructured.NestedString(obj.Object, "spec", "appSecret")
	founderSaName, _, _ := unstructured.NestedString(obj.Object, "spec", "founderSaName")
	license, _, _ := unstructured.NestedString(obj.Object, "spec", "license")
	return LicenseCRDSpec{
		AppId:         appId,
		AppSecret:     appSecret,
		FounderSaName: founderSaName,
		License:       license,
	}
}

func NewOverSellingConfigCRD(name string, spec OverSellingConfigCRDSpec) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ConfigCRDGroup + "/" + ConfigCRDVersion)
	obj.SetKind("OverSellingConfig")
	obj.SetName(name)
	obj.Object["spec"] = map[string]interface{}{
		"cpu":          int64(spec.CPU),
		"memory":       int64(spec.Memory),
		"storage":      int64(spec.Storage),
		"bandwidth":    int64(spec.BandWidth),
		"bandwidthNum": int64(spec.BandWidthNum),
	}
	return obj
}

func ParseOverSellingConfigCRDSpec(obj *unstructured.Unstructured) OverSellingConfigCRDSpec {
	cpu, _, _ := unstructured.NestedInt64(obj.Object, "spec", "cpu")
	memory, _, _ := unstructured.NestedInt64(obj.Object, "spec", "memory")
	storage, _, _ := unstructured.NestedInt64(obj.Object, "spec", "storage")
	bandwidth, _, _ := unstructured.NestedInt64(obj.Object, "spec", "bandwidth")
	bandwidthNum, _, _ := unstructured.NestedInt64(obj.Object, "spec", "bandwidthNum")
	return OverSellingConfigCRDSpec{
		CPU:          int32(cpu),
		Memory:       int32(memory),
		Storage:      int32(storage),
		BandWidth:    int32(bandwidth),
		BandWidthNum: int32(bandwidthNum),
	}
}

func NewFilingConfigCRD(name string, spec FilingConfigCRDSpec) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ConfigCRDGroup + "/" + ConfigCRDVersion)
	obj.SetKind("FilingConfig")
	obj.SetName(name)
	obj.Object["spec"] = map[string]interface{}{
		"icpnumber": spec.IcpNumber,
		"number":    spec.Number,
		"location":  spec.Location,
		"license":   spec.License,
		"tbol":      spec.Tbol,
	}
	return obj
}

func ParseFilingConfigCRDSpec(obj *unstructured.Unstructured) FilingConfigCRDSpec {
	icpNumber, _, _ := unstructured.NestedString(obj.Object, "spec", "icpnumber")
	number, _, _ := unstructured.NestedString(obj.Object, "spec", "number")
	location, _, _ := unstructured.NestedString(obj.Object, "spec", "location")
	license, _, _ := unstructured.NestedString(obj.Object, "spec", "license")
	tbol, _, _ := unstructured.NestedString(obj.Object, "spec", "tbol")
	return FilingConfigCRDSpec{
		IcpNumber: icpNumber,
		Number:    number,
		Location:  location,
		License:   license,
		Tbol:      tbol,
	}
}

func NewDomainParseConfigCRD(name string, spec DomainParseConfigCRDSpec) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ConfigCRDGroup + "/" + ConfigCRDVersion)
	obj.SetKind("DomainParseConfig")
	obj.SetName(name)
	obj.Object["spec"] = map[string]interface{}{
		"type":  spec.Type,
		"ips":   spec.IPs,
		"cname": spec.Cname,
	}
	return obj
}

func ParseDomainParseConfigCRDSpec(obj *unstructured.Unstructured) DomainParseConfigCRDSpec {
	recordType, _, _ := unstructured.NestedString(obj.Object, "spec", "type")
	ips, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "ips")
	cname, _, _ := unstructured.NestedString(obj.Object, "spec", "cname")
	return DomainParseConfigCRDSpec{
		Type:  recordType,
		IPs:   ips,
		Cname: cname,
	}
}

func NewContactConfigCRD(name string, spec ContactConfigCRDSpec) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(ConfigCRDGroup + "/" + ConfigCRDVersion)
	obj.SetKind("ContactConfig")
	obj.SetName(name)
	obj.Object["spec"] = map[string]interface{}{
		"type":     spec.Type,
		"link":     spec.Link,
		"text":     spec.Text,
		"name":     spec.Name,
		"showName": spec.ShowName,
		"selicon":  spec.SelIcon,
		"icon":     spec.Icon,
		"qrcode":   spec.Qrcode,
		"style":    spec.Style,
		"index":    int64(spec.Index),
	}
	return obj
}

func ParseContactConfigCRDSpec(obj *unstructured.Unstructured) ContactConfigCRDSpec {
	contactType, _, _ := unstructured.NestedString(obj.Object, "spec", "type")
	link, _, _ := unstructured.NestedString(obj.Object, "spec", "link")
	text, _, _ := unstructured.NestedString(obj.Object, "spec", "text")
	name, _, _ := unstructured.NestedString(obj.Object, "spec", "name")
	showName, _, _ := unstructured.NestedBool(obj.Object, "spec", "showName")
	selIcon, _, _ := unstructured.NestedString(obj.Object, "spec", "selicon")
	icon, _, _ := unstructured.NestedString(obj.Object, "spec", "icon")
	qrcode, _, _ := unstructured.NestedString(obj.Object, "spec", "qrcode")
	style, _, _ := unstructured.NestedString(obj.Object, "spec", "style")
	index, _, _ := unstructured.NestedInt64(obj.Object, "spec", "index")
	return ContactConfigCRDSpec{
		Type:     contactType,
		Link:     link,
		Text:     text,
		Name:     name,
		ShowName: showName,
		SelIcon:  selIcon,
		Icon:     icon,
		Qrcode:   qrcode,
		Style:    style,
		Index:    int32(index),
	}
}

func (self *Sdk) GetConfigCRD(ctx context.Context, gvr schema.GroupVersionResource, name string) (*unstructured.Unstructured, error) {
	return self.dynamicClient.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
}

func (self *Sdk) GetConfigCRDData(ctx context.Context, gvr schema.GroupVersionResource, name string) (map[string]string, error) {
	obj, err := self.GetConfigCRD(ctx, gvr, name)
	if err != nil {
		return nil, err
	}
	return ConfigCRDData(obj), nil
}
