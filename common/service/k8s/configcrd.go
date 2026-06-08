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
	K3kConfigName         = "k3k.config"
	K3sConfigName         = "k3s.config"
	LicenseName           = "license"
	OverSellingConfigName = "k3k.overselling.config"
)

var (
	K3kConfigGVR         = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "k3kconfigs"}
	K3sConfigGVR         = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "k3sconfigs"}
	LicenseGVR           = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "licenses"}
	OverSellingConfigGVR = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "oversellingconfigs"}
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
