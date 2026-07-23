package console

import (
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/k8s"
	userv1alpha1 "github.com/w7panel/w7panel/k8s/pkg/apis/user/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type LicenseClient struct {
	w7config config.W7ConfigRepositoryInterface
	sdk      *k8s.Sdk
}

var licenseClient *LicenseClient

func init() {
	licenseClient, _ = NewDefaultLicenseClient()
}

func NewDefaultLicenseClient() (*LicenseClient, error) {
	sdk := k8s.NewK8sClient().Sdk
	if sdk == nil {
		return nil, errors.New("sdk is nil")
	}
	api := NewLicenseClient(config.NewW7ConfigRepository(sdk), sdk)
	return api, nil
}

func NewLicenseClient(api config.W7ConfigRepositoryInterface, sdk *k8s.Sdk) *LicenseClient {
	return &LicenseClient{w7config: api, sdk: sdk}
}

func (api *LicenseClient) GetLicense() (*License, error) {
	license, err := api.getUserLicense()
	if err == nil {
		return license, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	obj, err := api.sdk.GetConfigCRD(api.sdk.Ctx, k8s.LicenseGVR, k8s.LicenseName)
	if err == nil {
		license, err := licenseFromCRDSpec(k8s.ParseLicenseCRDSpec(obj))
		if err != nil {
			return nil, err
		}
		if err := api.SetLicense(license); err != nil {
			return nil, err
		}
		return license, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	secret, err := api.sdk.GetLicense()
	if err != nil {
		return nil, err
	}
	license, err = licenseFromSecret(secret)
	if err != nil {
		return nil, err
	}
	if err := api.SetLicense(license); err != nil {
		return nil, err
	}
	return license, nil
}

func (api *LicenseClient) getUserLicense() (*License, error) {
	list, err := api.sdk.DynamicClient().Resource(userv1alpha1.GVR).List(api.sdk.Ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for i := range list.Items {
		spec, ok, err := userLicenseSpec(&list.Items[i])
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		return licenseFromCRDSpec(spec)
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{Group: userv1alpha1.GVR.Group, Resource: userv1alpha1.GVR.Resource}, k8s.LicenseName)
}

func licenseFromSecret(secret *corev1.Secret) (*License, error) {
	var x509Certificate *x509.Certificate = nil
	if len(secret.Data["license"]) > 0 {
		cert, err := helper.ParseX509(secret.Data["license"])
		if err == nil {
			x509Certificate = cert
		}
	}
	return &License{
		AppId:         string(secret.Data["appId"]),
		AppSecret:     string(secret.Data["appSecret"]),
		License:       x509Certificate,
		FounderSaName: string(secret.Data["founderSaName"]),
	}, nil
}

func licenseFromCRDSpec(spec k8s.LicenseCRDSpec) (*License, error) {
	var x509Certificate *x509.Certificate = nil
	if spec.License != "" {
		certData, err := base64.StdEncoding.DecodeString(spec.License)
		if err != nil {
			return nil, err
		}
		cert, err := helper.ParseX509(certData)
		if err != nil {
			return nil, err
		}
		x509Certificate = cert
	}
	return &License{
		AppId:         spec.AppId,
		AppSecret:     spec.AppSecret,
		License:       x509Certificate,
		FounderSaName: spec.FounderSaName,
	}, nil
}

func licenseCRDSpec(license *License) k8s.LicenseCRDSpec {
	spec := k8s.LicenseCRDSpec{
		AppId:         license.AppId,
		AppSecret:     license.AppSecret,
		FounderSaName: license.FounderSaName,
	}
	if license.License != nil {
		spec.License = base64.StdEncoding.EncodeToString(license.License.Raw)
	}
	return spec
}

func userLicenseSpec(obj *unstructured.Unstructured) (k8s.LicenseCRDSpec, bool, error) {
	spec, ok, err := unstructured.NestedMap(obj.Object, "spec", "license")
	if err != nil || !ok {
		return k8s.LicenseCRDSpec{}, ok, err
	}
	return k8s.LicenseCRDSpec{
		AppId:         stringValue(spec["appId"]),
		AppSecret:     stringValue(spec["appSecret"]),
		FounderSaName: stringValue(spec["founderSaName"]),
		License:       stringValue(spec["license"]),
	}, true, nil
}

func userLicenseSpecMap(spec k8s.LicenseCRDSpec) map[string]interface{} {
	return map[string]interface{}{
		"appId":         spec.AppId,
		"appSecret":     spec.AppSecret,
		"founderSaName": spec.FounderSaName,
		"license":       spec.License,
	}
}

func stringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func (api *LicenseClient) SetLicense(license *License) error {
	return api.setLicenseClean(license, false)
}

func (api *LicenseClient) CleanLicense() error {
	license, err := api.GetLicense()
	if err != nil {
		return err
	}
	return api.setLicenseClean(license, true)
}

func (api *LicenseClient) setLicenseClean(license *License, cleanCert bool) error {
	if license == nil {
		return errors.New("license is nil")
	}
	if license.FounderSaName == "" {
		return errors.New("founder service account name is empty")
	}
	spec := licenseCRDSpec(license)
	if cleanCert {
		spec.License = ""
	}
	obj, err := api.sdk.DynamicClient().Resource(userv1alpha1.GVR).Get(api.sdk.Ctx, license.FounderSaName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedMap(obj.Object, userLicenseSpecMap(spec), "spec", "license"); err != nil {
		return fmt.Errorf("failed to set user license spec: %w", err)
	}
	_, err = api.sdk.DynamicClient().Resource(userv1alpha1.GVR).Update(api.sdk.Ctx, obj, metav1.UpdateOptions{})
	return err
}

func (c *LicenseClient) CreateLicenseSite(userName string, ignoreExits bool) (*License, error) {
	if !ignoreExits {
		license, err := c.GetLicense()
		if err == nil {
			return license, err
		}
	}

	clusterUniquId, err := c.sdk.GetClusterId()
	if err != nil {
		return nil, err
	}

	config, err := c.w7config.Get(userName)
	if err != nil {
		slog.Error("获取配置失败")
		return nil, err
	}
	token := config.ThirdpartyCDToken
	api := NewConsoleCdClient(token)
	data := map[string]string{
		"offlineUrl": config.OfflineUrl,
		"sn":         clusterUniquId,
	}
	license, err := api.CreateLicenseSite(data)
	if err != nil {
		return nil, err
	}
	license.FounderSaName = userName //创始人账号
	SetCurrentLicense(license)
	// console.License = license
	err = c.SetLicense(license)
	if err != nil {
		return nil, err
	}
	return license, nil

}

func (c *LicenseClient) ImportCert(pemData []byte, saName string) error {
	license, err := c.GetLicense()
	if err != nil {
		return err
	}

	cert, err := helper.ParseX509(pemData)
	if err != nil {
		return err
	}
	// if cert.Issuer.Organization != nil && len(cert.Issuer.Organization) > 0 {
	// 	if cert.Issuer.Organization[0] != license.AppId {
	// 		return errors.New("证书颁发者与授权不符")
	// 	}
	// }
	_, err = VerifyCert(cert)
	if err != nil {
		return err
	}
	// 兼容w7config license先
	w7config, err := c.w7config.Get(saName)
	if err != nil {
		slog.Error("获取配置失败")
	}
	w7config.License = cert
	err = c.w7config.Set(w7config)
	if err != nil {
		return err
	}
	config.MainW7Config = w7config
	license.License = cert
	// config.LicenseVerify = true
	if cert != nil && len(cert.Subject.Province) > 0 {
		// config.LicenseType = cert.Subject.Province[0]
		// config.SetVerifyType(w7config.Name, cert.Subject.Province[0])

	}

	err = c.SetLicense(license)
	if err != nil {
		return err
	}
	SetCurrentLicense(license)
	return nil

}

func (c *LicenseClient) GetConfig(saName string) (*config.W7Config, error) {
	return c.w7config.Get(saName)
}
