package coredns

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/caddy/caddyfile"
	"github.com/w7panel/w7panel/common/service/k8s"
	"golang.org/x/mod/semver"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const (
	filename                    = "Caddyfile"
	coreDNSCorefileKey          = "Corefile"
	coreDNSCustomImport         = "import /etc/coredns/custom/*.server"
	coreDNSCustomVolumeName     = "w7-coredns-custom"
	coreDNSCustomMountPath      = "/etc/coredns/custom"
	coreDNSRestartAnnotationKey = "w7.cc/coredns-restarted-at"
)

type CoreDnsController struct {
	*caddy.Controller
}

type Service struct {
	sdk       *k8s.Sdk
	clientSet kubernetes.Interface
}

func NewService() *Service {
	sdk := k8s.NewK8sClient().Sdk
	return &Service{sdk: sdk, clientSet: sdk.ClientSet}
}

func NewServiceWithSdk(sdk *k8s.Sdk) *Service {
	return &Service{sdk: sdk, clientSet: sdk.ClientSet}
}

func newServiceWithClient(clientSet kubernetes.Interface) *Service {
	return &Service{clientSet: clientSet}
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
	changed := false
	for key := range cfg.Data {
		domain, ok := DomainFromConfigMapKey(key)
		if !ok {
			continue
		}
		data, itemChanged, err := ensureZoneData(cfg, domain)
		if err != nil {
			return nil, err
		}
		changed = changed || itemChanged
		records, _ := ParseZone(domain, data)
		zones = append(zones, Zone{
			Domain:     domain,
			RecordNum:  len(records),
			UpdateTime: configMapTime(cfg),
		})
	}
	if changed {
		if _, err := s.updateCustomConfigMap(ctx, cfg); err != nil {
			return nil, err
		}
		if err := s.applyCoreDNSChange(ctx); err != nil {
			return nil, err
		}
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
	serverKey := ConfigMapKey(domain)
	zoneKey := ZoneFileConfigMapKey(domain)
	if _, ok := cfg.Data[serverKey]; ok {
		if _, changed, err := ensureZoneData(cfg, domain); err != nil {
			return Zone{}, err
		} else if changed {
			if _, err := s.updateCustomConfigMap(ctx, cfg); err != nil {
				return Zone{}, err
			}
			if err := s.applyCoreDNSChange(ctx); err != nil {
				return Zone{}, err
			}
		}
		return Zone{Domain: domain, UpdateTime: configMapTime(cfg)}, nil
	}
	serverData, err := RenderZoneServer(domain)
	if err != nil {
		return Zone{}, err
	}
	zoneData, err := RenderZone(domain, nil)
	if err != nil {
		return Zone{}, err
	}
	cfg.Data[serverKey] = serverData
	cfg.Data[zoneKey] = zoneData
	if _, err := s.updateCustomConfigMap(ctx, cfg); err != nil {
		return Zone{}, err
	}
	if err := s.applyCoreDNSChange(ctx); err != nil {
		return Zone{}, err
	}
	return Zone{Domain: domain}, nil
}

func (s *Service) Info(ctx context.Context) (Info, error) {
	info := Info{FileFallthroughMinVersion: CoreDNSMinVersion}
	deployment, err := s.clientSet.AppsV1().Deployments(CoreDNSNamespace).Get(ctx, CoreDNSName, metav1.GetOptions{})
	if err != nil {
		return info, err
	}
	info.Image = string(deployment.Spec.Template.Spec.Containers[0].Image)
	if info.Image != "" {
		parts := strings.Split(info.Image, ":")
		info.Version = parts[len(parts)-1]
		if !strings.HasPrefix(info.Version, "v") {
			info.Version = "v" + info.Version
		}
	}
	info.FileFallthroughSupported = semver.Compare(info.Version, CoreDNSMinVersion) >= 0
	if !info.FileFallthroughSupported {
		info.FileFallthroughMessage = CoreDNSFallthroughUnsupportedMessage
	}
	return info, nil
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
	delete(cfg.Data, ZoneFileConfigMapKey(domain))
	if _, err = s.updateCustomConfigMap(ctx, cfg); err != nil {
		return err
	}
	return s.applyCoreDNSChange(ctx)
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
	serverKey := ConfigMapKey(domain)
	zoneKey := ZoneFileConfigMapKey(domain)
	if _, ok := cfg.Data[serverKey]; !ok {
		return Record{}, apierrors.NewNotFound(corev1.Resource("dnszone"), domain)
	}
	zoneData, _, err := ensureZoneData(cfg, domain)
	if err != nil {
		return Record{}, err
	}
	records, _ := ParseZone(domain, zoneData)
	records = append(records, record)
	data, err := RenderZoneWithNextSerial(domain, records, zoneData)
	if err != nil {
		return Record{}, err
	}
	serverData, err := RenderZoneServer(domain)
	if err != nil {
		return Record{}, err
	}
	cfg.Data[serverKey] = serverData
	cfg.Data[zoneKey] = data
	if _, err := s.updateCustomConfigMap(ctx, cfg); err != nil {
		return Record{}, err
	}
	if err := s.applyCoreDNSChange(ctx); err != nil {
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
	serverKey := ConfigMapKey(domain)
	zoneKey := ZoneFileConfigMapKey(domain)
	if _, ok := cfg.Data[serverKey]; !ok {
		return Record{}, apierrors.NewNotFound(corev1.Resource("dnszone"), domain)
	}
	zoneData, _, err := ensureZoneData(cfg, domain)
	if err != nil {
		return Record{}, err
	}
	records, err := ParseZone(domain, zoneData)
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
	data, err := RenderZoneWithNextSerial(domain, records, zoneData)
	if err != nil {
		return Record{}, err
	}
	serverData, err := RenderZoneServer(domain)
	if err != nil {
		return Record{}, err
	}
	cfg.Data[serverKey] = serverData
	cfg.Data[zoneKey] = data
	if _, err := s.updateCustomConfigMap(ctx, cfg); err != nil {
		return Record{}, err
	}
	if err := s.applyCoreDNSChange(ctx); err != nil {
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
	serverKey := ConfigMapKey(domain)
	zoneKey := ZoneFileConfigMapKey(domain)
	if _, ok := cfg.Data[serverKey]; !ok {
		return apierrors.NewNotFound(corev1.Resource("dnszone"), domain)
	}
	zoneData, _, err := ensureZoneData(cfg, domain)
	if err != nil {
		return err
	}
	records, err := ParseZone(domain, zoneData)
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
	data, err := RenderZoneWithNextSerial(domain, next, zoneData)
	if err != nil {
		return err
	}
	serverData, err := RenderZoneServer(domain)
	if err != nil {
		return err
	}
	cfg.Data[serverKey] = serverData
	cfg.Data[zoneKey] = data
	if _, err = s.updateCustomConfigMap(ctx, cfg); err != nil {
		return err
	}
	return s.applyCoreDNSChange(ctx)
}

func (s *Service) ServerStatus(ctx context.Context) (ServerStatus, error) {
	status := ServerStatus{ServiceName: PublicDNSServiceName}
	svc, err := s.clientSet.CoreV1().Services(CoreDNSNamespace).Get(ctx, PublicDNSServiceName, metav1.GetOptions{})
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
	services := s.clientSet.CoreV1().Services(CoreDNSNamespace)
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
	if _, ok := cfg.Data[ConfigMapKey(domain)]; !ok {
		return "", "", apierrors.NewNotFound(corev1.Resource("dnszone"), domain)
	}
	data, changed, err := ensureZoneData(cfg, domain)
	if err != nil {
		return "", "", err
	}
	if changed {
		if _, err := s.updateCustomConfigMap(ctx, cfg); err != nil {
			return "", "", err
		}
		if err := s.applyCoreDNSChange(ctx); err != nil {
			return "", "", err
		}
	}
	return domain, data, nil
}

func ensureZoneData(cfg *corev1.ConfigMap, domain string) (string, bool, error) {
	domain, err := NormalizeDomain(domain)
	if err != nil {
		return "", false, err
	}
	if cfg.Data == nil {
		cfg.Data = map[string]string{}
	}
	serverKey := ConfigMapKey(domain)
	zoneKey := ZoneFileConfigMapKey(domain)
	server, ok := cfg.Data[serverKey]
	if !ok {
		return "", false, apierrors.NewNotFound(corev1.Resource("dnszone"), domain)
	}
	serverData, err := RenderZoneServer(domain)
	if err != nil {
		return "", false, err
	}
	changed := false
	if cfg.Data[serverKey] != serverData {
		cfg.Data[serverKey] = serverData
		changed = true
	}
	if data, ok := cfg.Data[zoneKey]; ok && strings.TrimSpace(data) != "" {
		return data, changed, nil
	}
	records, err := ParseLegacyTemplateZone(domain, server)
	if err != nil {
		return "", false, err
	}
	data, err := RenderZone(domain, records)
	if err != nil {
		return "", false, err
	}
	cfg.Data[zoneKey] = data
	return data, true, nil
}

func (s *Service) getOrCreateCustomConfigMap(ctx context.Context) (*corev1.ConfigMap, error) {
	configMaps := s.clientSet.CoreV1().ConfigMaps(CoreDNSNamespace)
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
	return s.clientSet.CoreV1().ConfigMaps(CoreDNSNamespace).Update(ctx, cfg, metav1.UpdateOptions{})
}

func (s *Service) applyCoreDNSChange(ctx context.Context) error {
	if err := s.ensureCoreDNSCorefileImportsCustomZones(ctx); err != nil {
		return err
	}
	deployment, err := s.getCoreDNSDeployment(ctx)
	if err != nil {
		return err
	}
	ensureCoreDNSDeploymentCustomConfig(deployment)
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = map[string]string{}
	}
	deployment.Spec.Template.Annotations[coreDNSRestartAnnotationKey] = time.Now().Format(time.RFC3339Nano)
	_, err = s.clientSet.AppsV1().Deployments(CoreDNSNamespace).Update(ctx, deployment, metav1.UpdateOptions{})
	return err
}

func (s *Service) ensureCoreDNSCorefileImportsCustomZones(ctx context.Context) error {
	configMaps := s.clientSet.CoreV1().ConfigMaps(CoreDNSNamespace)
	cfg, err := configMaps.Get(ctx, CoreDNSName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return apierrors.NewNotFound(corev1.Resource("configmap"), CoreDNSName)
		}
		return err
	}
	corefile, ok := cfg.Data[coreDNSCorefileKey]
	if !ok {
		return fmt.Errorf("%s/%s configmap missing %q", CoreDNSNamespace, CoreDNSName, coreDNSCorefileKey)
	}
	nextCorefile := ensureCoreDNSCustomImport(corefile)
	if nextCorefile == corefile {
		return nil
	}
	if cfg.Data == nil {
		cfg.Data = map[string]string{}
	}
	cfg.Data[coreDNSCorefileKey] = nextCorefile
	_, err = configMaps.Update(ctx, cfg, metav1.UpdateOptions{})
	return err
}

func (s *Service) getCoreDNSDeployment(ctx context.Context) (*appsv1.Deployment, error) {
	deployments := s.clientSet.AppsV1().Deployments(CoreDNSNamespace)
	deployment, err := deployments.Get(ctx, CoreDNSName, metav1.GetOptions{})
	if err == nil {
		return deployment, nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	list, err := deployments.List(ctx, metav1.ListOptions{LabelSelector: "k8s-app=kube-dns"})
	if err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, apierrors.NewNotFound(corev1.Resource("deployment"), CoreDNSName)
	}
	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].Name < list.Items[j].Name
	})
	return &list.Items[0], nil
}

func ensureCoreDNSCustomImport(corefile string) string {
	for _, line := range strings.Split(corefile, "\n") {
		if strings.TrimSpace(line) == coreDNSCustomImport {
			return corefile
		}
	}
	corefile = strings.TrimRight(corefile, "\n")
	if corefile == "" {
		return coreDNSCustomImport + "\n"
	}
	return corefile + "\n" + coreDNSCustomImport + "\n"
}

func ensureCoreDNSDeploymentCustomConfig(deployment *appsv1.Deployment) {
	optional := true
	customVolume := corev1.Volume{
		Name: coreDNSCustomVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: CoreDNSCustomName},
				Optional:             &optional,
			},
		},
	}
	volumeFound := false
	for i := range deployment.Spec.Template.Spec.Volumes {
		if deployment.Spec.Template.Spec.Volumes[i].Name == coreDNSCustomVolumeName {
			deployment.Spec.Template.Spec.Volumes[i] = customVolume
			volumeFound = true
			break
		}
	}
	if !volumeFound {
		deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{
			Name:         customVolume.Name,
			VolumeSource: customVolume.VolumeSource,
		})
	}
	for i := range deployment.Spec.Template.Spec.Containers {
		mounts := deployment.Spec.Template.Spec.Containers[i].VolumeMounts
		mountFound := false
		for j := range mounts {
			if mounts[j].Name == coreDNSCustomVolumeName {
				mounts[j].MountPath = coreDNSCustomMountPath
				mounts[j].ReadOnly = true
				mountFound = true
				break
			}
			if mounts[j].MountPath == coreDNSCustomMountPath {
				mountFound = true
			}
		}
		if mountFound {
			deployment.Spec.Template.Spec.Containers[i].VolumeMounts = mounts
			continue
		}
		deployment.Spec.Template.Spec.Containers[i].VolumeMounts = append(mounts,
			corev1.VolumeMount{
				Name:      coreDNSCustomVolumeName,
				MountPath: coreDNSCustomMountPath,
				ReadOnly:  true,
			},
		)
	}
}

func hasCoreDNSCustomVolume(volumes []corev1.Volume) bool {
	for _, volume := range volumes {
		if volume.Name == coreDNSCustomVolumeName {
			return true
		}
	}
	return false
}

func hasCoreDNSCustomVolumeMount(mounts []corev1.VolumeMount) bool {
	for _, mount := range mounts {
		if mount.Name == coreDNSCustomVolumeName || mount.MountPath == coreDNSCustomMountPath {
			return true
		}
	}
	return false
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
