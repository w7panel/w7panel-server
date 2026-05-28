package coredns

import (
	"bytes"
	"context"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/caddy/caddyfile"
	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const filename = "Caddyfile"

type CoreDnsController struct {
	*caddy.Controller
}

type Service struct {
	sdk *k8s.Sdk
}

func NewService() *Service {
	return &Service{sdk: k8s.NewK8sClient().Sdk}
}

func NewServiceWithSdk(sdk *k8s.Sdk) *Service {
	return &Service{sdk: sdk}
}

func NewTestController(serverType, input string) *CoreDnsController {
	c := caddy.NewTestController(serverType, input)
	return &CoreDnsController{
		c,
	}
}

func (s *Service) ListZones(ctx context.Context) ([]Zone, error) {
	cfg, err := s.getOrCreateCustomConfigMap(ctx)
	if err != nil {
		return nil, err
	}
	zones := make([]Zone, 0)
	for key, data := range cfg.Data {
		domain, ok := DomainFromConfigMapKey(key)
		if !ok {
			continue
		}
		records, _ := ParseZone(domain, data)
		zones = append(zones, Zone{
			Domain:     domain,
			RecordNum:  len(records),
			UpdateTime: configMapTime(cfg),
		})
	}
	return zones, nil
}

func (s *Service) CreateZone(ctx context.Context, domain string) (Zone, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return Zone{}, err
	}
	cfg, err := s.getOrCreateCustomConfigMap(ctx)
	if err != nil {
		return Zone{}, err
	}
	if cfg.Data == nil {
		cfg.Data = map[string]string{}
	}
	key := ConfigMapKey(domain)
	if _, ok := cfg.Data[key]; ok {
		return Zone{Domain: domain, UpdateTime: configMapTime(cfg)}, nil
	}
	data, err := RenderZone(domain, nil)
	if err != nil {
		return Zone{}, err
	}
	cfg.Data[key] = data
	if _, err := s.updateCustomConfigMap(ctx, cfg); err != nil {
		return Zone{}, err
	}
	return Zone{Domain: domain}, nil
}

func (s *Service) DeleteZone(ctx context.Context, domain string) error {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return err
	}
	cfg, err := s.getOrCreateCustomConfigMap(ctx)
	if err != nil {
		return err
	}
	delete(cfg.Data, ConfigMapKey(domain))
	_, err = s.updateCustomConfigMap(ctx, cfg)
	return err
}

func (s *Service) ListRecords(ctx context.Context, domain string) ([]Record, error) {
	domain, data, err := s.getZoneData(ctx, domain)
	if err != nil {
		return nil, err
	}
	return ParseZone(domain, data)
}

func (s *Service) CreateRecord(ctx context.Context, domain string, record Record) (Record, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return Record{}, err
	}
	record, err = NormalizeRecord(domain, record)
	if err != nil {
		return Record{}, err
	}
	cfg, err := s.getOrCreateCustomConfigMap(ctx)
	if err != nil {
		return Record{}, err
	}
	key := ConfigMapKey(domain)
	if _, ok := cfg.Data[key]; !ok {
		return Record{}, apierrors.NewNotFound(corev1.Resource("configmap"), key)
	}
	records, _ := ParseZone(domain, cfg.Data[key])
	records = append(records, record)
	data, err := RenderZone(domain, records)
	if err != nil {
		return Record{}, err
	}
	cfg.Data[key] = data
	if _, err := s.updateCustomConfigMap(ctx, cfg); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Service) UpdateRecord(ctx context.Context, domain string, id string, record Record) (Record, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return Record{}, err
	}
	record.ID = id
	record, err = NormalizeRecord(domain, record)
	if err != nil {
		return Record{}, err
	}
	cfg, err := s.getOrCreateCustomConfigMap(ctx)
	if err != nil {
		return Record{}, err
	}
	key := ConfigMapKey(domain)
	records, err := ParseZone(domain, cfg.Data[key])
	if err != nil {
		return Record{}, err
	}
	found := false
	for i := range records {
		if records[i].ID == id {
			records[i] = record
			found = true
			break
		}
	}
	if !found {
		return Record{}, apierrors.NewNotFound(corev1.Resource("dnsrecord"), id)
	}
	data, err := RenderZone(domain, records)
	if err != nil {
		return Record{}, err
	}
	cfg.Data[key] = data
	if _, err := s.updateCustomConfigMap(ctx, cfg); err != nil {
		return Record{}, err
	}
	return record, nil
}

func (s *Service) DeleteRecord(ctx context.Context, domain string, id string) error {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return err
	}
	cfg, err := s.getOrCreateCustomConfigMap(ctx)
	if err != nil {
		return err
	}
	key := ConfigMapKey(domain)
	records, err := ParseZone(domain, cfg.Data[key])
	if err != nil {
		return err
	}
	next := make([]Record, 0, len(records))
	found := false
	for _, record := range records {
		if record.ID == id {
			found = true
			continue
		}
		next = append(next, record)
	}
	if !found {
		return apierrors.NewNotFound(corev1.Resource("dnsrecord"), id)
	}
	data, err := RenderZone(domain, next)
	if err != nil {
		return err
	}
	cfg.Data[key] = data
	_, err = s.updateCustomConfigMap(ctx, cfg)
	return err
}

func (s *Service) ServerStatus(ctx context.Context) (ServerStatus, error) {
	status := ServerStatus{ServiceName: PublicDNSServiceName}
	svc, err := s.sdk.ClientSet.CoreV1().Services(CoreDNSNamespace).Get(ctx, PublicDNSServiceName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return status, nil
		}
		return status, err
	}
	status.Enabled = true
	status.ServiceType = string(svc.Spec.Type)
	ingress := make([]string, 0, len(svc.Status.LoadBalancer.Ingress))
	for _, item := range svc.Status.LoadBalancer.Ingress {
		if item.IP != "" {
			ingress = append(ingress, item.IP)
		}
		if item.Hostname != "" {
			ingress = append(ingress, item.Hostname)
		}
	}
	status.ExternalIPs = CollectServiceExternalIPs(ingress, svc.Spec.ExternalIPs)
	return status, nil
}

func (s *Service) SetServerEnabled(ctx context.Context, enabled bool) (ServerStatus, error) {
	services := s.sdk.ClientSet.CoreV1().Services(CoreDNSNamespace)
	if !enabled {
		err := services.Delete(ctx, PublicDNSServiceName, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return ServerStatus{}, err
		}
		return s.ServerStatus(ctx)
	}
	svc, err := services.Get(ctx, PublicDNSServiceName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return ServerStatus{}, err
		}
		svc = publicDNSService()
		if _, err := services.Create(ctx, svc, metav1.CreateOptions{}); err != nil {
			return ServerStatus{}, err
		}
		return s.ServerStatus(ctx)
	}
	svc.Spec.Type = corev1.ServiceTypeLoadBalancer
	svc.Spec.Selector = map[string]string{"k8s-app": "kube-dns"}
	svc.Spec.Ports = dnsServicePorts()
	if _, err := services.Update(ctx, svc, metav1.UpdateOptions{}); err != nil {
		return ServerStatus{}, err
	}
	return s.ServerStatus(ctx)
}

func (s *Service) getZoneData(ctx context.Context, domain string) (string, string, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return "", "", err
	}
	cfg, err := s.getOrCreateCustomConfigMap(ctx)
	if err != nil {
		return "", "", err
	}
	data, ok := cfg.Data[ConfigMapKey(domain)]
	if !ok {
		return "", "", apierrors.NewNotFound(corev1.Resource("dnszone"), domain)
	}
	return domain, data, nil
}

func (s *Service) getOrCreateCustomConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	configMaps := s.sdk.ClientSet.CoreV1().ConfigMaps(CoreDNSNamespace)
	cfg, err := configMaps.Get(ctx, CoreDNSCustomName, metav1.GetOptions{})
	if err == nil {
		if cfg.Data == nil {
			cfg.Data = map[string]string{}
		}
		return cfg, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}
	return configMaps.Create(ctx, &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      CoreDNSCustomName,
			Namespace: CoreDNSNamespace,
		},
		Data: map[string]string{},
	}, metav1.CreateOptions{})
}

func (s *Service) updateCustomConfigMap(ctx context.Context, cfg *corev1.ConfigMap) (*corev1.ConfigMap, error) {
	return s.sdk.ClientSet.CoreV1().ConfigMaps(CoreDNSNamespace).Update(ctx, cfg, metav1.UpdateOptions{})
}

func publicDNSService() *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      PublicDNSServiceName,
			Namespace: CoreDNSNamespace,
			Labels: map[string]string{
				"w7.cc/managed-by": "w7panel",
				"w7.cc/component":  "dns",
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{"k8s-app": "kube-dns"},
			Ports:    dnsServicePorts(),
		},
	}
}

func dnsServicePorts() []corev1.ServicePort {
	return []corev1.ServicePort{
		{
			Name:       "dns-udp",
			Protocol:   corev1.ProtocolUDP,
			Port:       53,
			TargetPort: intstr.FromInt(53),
		},
		{
			Name:       "dns-tcp",
			Protocol:   corev1.ProtocolTCP,
			Port:       53,
			TargetPort: intstr.FromInt(53),
		},
	}
}

func configMapTime(cfg *corev1.ConfigMap) time.Time {
	latest := cfg.CreationTimestamp.Time
	for _, item := range cfg.ManagedFields {
		if item.Time != nil && item.Time.After(latest) {
			latest = item.Time.Time
		}
	}
	return latest
}

func ParseConfig() ([]caddyfile.ServerBlock, error) {
	sdk := k8s.NewK8sClient()
	cfg, err := sdk.ClientSet.CoreV1().ConfigMaps(CoreDNSNamespace).Get(sdk.Ctx, CoreDNSCustomName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	data := cfg.Data["demo.server"]
	serverBlocks, err := caddyfile.Parse(filename, bytes.NewReader([]byte(data)), nil)
	if err != nil {
		return nil, err
	}
	return serverBlocks, nil
}

func ParseToJsonConfig() ([]byte, error) {
	sdk := k8s.NewK8sClient()
	cfg, err := sdk.ClientSet.CoreV1().ConfigMaps(CoreDNSNamespace).Get(sdk.Ctx, CoreDNSCustomName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	data := cfg.Data["demo.server"]
	serverBlocks, err := caddyfile.ToJSON([]byte(data))
	if err != nil {
		return nil, err
	}
	return serverBlocks, nil
}
