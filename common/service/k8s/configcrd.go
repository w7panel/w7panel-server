package k8s

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	ConfigCRDGroup   = "w7panel.w7.com"
	ConfigCRDVersion = "v1alpha1"
	K3kConfigName    = "k3k.config"
	K3sConfigName    = "k3s.config"
	LicenseName      = "license"
)

var (
	K3kConfigGVR = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "k3kconfigs"}
	K3sConfigGVR = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "k3sconfigs"}
	LicenseGVR   = schema.GroupVersionResource{Group: ConfigCRDGroup, Version: ConfigCRDVersion, Resource: "licenses"}
)

type LicenseCRDSpec struct {
	AppId         string
	AppSecret     string
	FounderSaName string
	License       string
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
