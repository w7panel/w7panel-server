package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	"golang.org/x/mod/semver"
	"helm.sh/helm/v3/pkg/strvals"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	defaultMaxConcurrent int32 = 3
	defaultMaxRetries    int32 = 3
)

var defaultArtifactTimeout = 10 * time.Minute

type effectiveProfile struct {
	MaxConcurrent      int32
	MaxRetries         int32
	TimeoutPerArtifact time.Duration
}

func profileSettings(profile *bootstrapv1.BootstrapProfile) effectiveProfile {
	settings := effectiveProfile{
		MaxConcurrent:      defaultMaxConcurrent,
		MaxRetries:         defaultMaxRetries,
		TimeoutPerArtifact: defaultArtifactTimeout,
	}
	if profile.Spec.Strategy.MaxConcurrent > 0 {
		settings.MaxConcurrent = profile.Spec.Strategy.MaxConcurrent
	}
	if profile.Spec.Strategy.MaxRetries != nil {
		settings.MaxRetries = *profile.Spec.Strategy.MaxRetries
	}
	if profile.Spec.Strategy.TimeoutPerArtifact.Duration > 0 {
		settings.TimeoutPerArtifact = profile.Spec.Strategy.TimeoutPerArtifact.Duration
	}
	return settings
}

func effectiveArtifact(profile *bootstrapv1.BootstrapProfile, artifact bootstrapv1.BootstrapArtifact) bootstrapv1.ArtifactInstallationSpec {
	failurePolicy := artifact.FailurePolicy
	if failurePolicy == "" {
		failurePolicy = profile.Spec.Defaults.FailurePolicy
	}
	if failurePolicy == "" {
		failurePolicy = bootstrapv1.FailurePolicyContinue
	}
	return bootstrapv1.ArtifactInstallationSpec{
		ProfileRef: bootstrapv1.BootstrapProfileReference{
			Name: profile.Name,
			UID:  string(profile.UID),
		},
		ProfileRevision: profile.Spec.Revision,
		Artifact: bootstrapv1.ArtifactReference{
			Name:      artifact.Name,
			Type:      effectiveArtifactType(artifact.Type),
			Identifie: artifact.Identifie,
			Source:    artifact.Source,
			Version:   artifact.Version,
		},
		Target: bootstrapv1.ArtifactTarget{
			ReleaseName: artifact.ReleaseName,
			Namespace:   artifact.Namespace,
		},
		FailurePolicy: failurePolicy,
		DependsOn:     append([]string(nil), artifact.DependsOn...),
		InstallOptions: bootstrapv1.BootstrapInstallOptions{
			HelmValues:  cloneStringMap(artifact.InstallOptions.HelmValues),
			Annotations: cloneStringMap(artifact.InstallOptions.Annotations),
		},
	}
}

func effectiveArtifactType(value bootstrapv1.ArtifactType) bootstrapv1.ArtifactType {
	if value == "" {
		return bootstrapv1.ArtifactTypeZPK
	}
	return value
}

func artifactInstallationName(profileName, artifactName string) string {
	name := profileName + "-" + artifactName
	if len(name) <= 253 {
		return name
	}
	sum := sha256.Sum256([]byte(name))
	return strings.TrimRight(name[:236], "-.") + "-" + hex.EncodeToString(sum[:8])
}

func operationID(installation *bootstrapv1.ArtifactInstallation) string {
	input := fmt.Sprintf("%s\x00%s\x00%s\x00%s",
		installation.Spec.ProfileRef.UID,
		installation.Spec.ProfileRevision,
		installation.Spec.Artifact.Name,
		installation.Spec.Artifact.Version,
	)
	sum := sha256.Sum256([]byte(input))
	// 128 bits keeps the ID collision-resistant and valid as a Kubernetes label value.
	return hex.EncodeToString(sum[:16])
}

func validateProfile(profile *bootstrapv1.BootstrapProfile) error {
	if profile.Spec.Revision == "" {
		return errors.New("spec.revision 不能为空")
	}
	if errs := validation.IsValidLabelValue(profile.Name); len(errs) > 0 {
		return fmt.Errorf("Profile 名称不能作为标签值: %s", strings.Join(errs, ", "))
	}
	settings := profileSettings(profile)
	if profile.Spec.Strategy.MaxConcurrent < 0 ||
		(profile.Spec.Strategy.MaxRetries != nil && *profile.Spec.Strategy.MaxRetries < 0) ||
		profile.Spec.Strategy.TimeoutPerArtifact.Duration < 0 {
		return errors.New("strategy 中的数值不能为负数")
	}
	if settings.MaxConcurrent > 50 {
		return errors.New("strategy.maxConcurrent 不能大于 50")
	}
	if err := validateFailurePolicy(profile.Spec.Defaults.FailurePolicy); err != nil {
		return fmt.Errorf("spec.defaults: %w", err)
	}

	byName := make(map[string]bootstrapv1.BootstrapArtifact, len(profile.Spec.Artifacts))
	targets := make(map[string]string, len(profile.Spec.Artifacts))
	for i, artifact := range profile.Spec.Artifacts {
		path := fmt.Sprintf("spec.artifacts[%d]", i)
		if artifact.Name == "" || artifact.Identifie == "" || artifact.Source == "" || artifact.ReleaseName == "" || artifact.Namespace == "" {
			return fmt.Errorf("%s 的 name、identifie、source、releaseName 和 namespace 均为必填项", path)
		}
		artifactType := effectiveArtifactType(artifact.Type)
		if artifactType != bootstrapv1.ArtifactTypeZPK {
			return fmt.Errorf("%s.type %q 当前不支持，仅 ZPK 执行器已启用", path, artifact.Type)
		}
		if errs := validation.IsDNS1123Label(artifact.Name); len(errs) > 0 {
			return fmt.Errorf("%s.name 无效: %s", path, strings.Join(errs, ", "))
		}
		if errs := validation.IsDNS1123Label(artifact.Namespace); len(errs) > 0 {
			return fmt.Errorf("%s.namespace 无效: %s", path, strings.Join(errs, ", "))
		}
		if errs := validation.IsDNS1123Subdomain(artifact.ReleaseName); len(errs) > 0 || len(artifact.ReleaseName) > 53 {
			return fmt.Errorf("%s.releaseName 必须是最长 53 字符的 DNS 子域名: %s", path, strings.Join(errs, ", "))
		}
		if errs := validation.IsValidLabelValue(normalizeIdentifie(artifact.Identifie)); len(errs) > 0 {
			return fmt.Errorf("%s.identifie 无效: %s", path, strings.Join(errs, ", "))
		}
		if _, exists := byName[artifact.Name]; exists {
			return fmt.Errorf("制品名称 %q 重复", artifact.Name)
		}
		byName[artifact.Name] = artifact
		target := artifact.Namespace + "/" + artifact.ReleaseName
		if previous, exists := targets[target]; exists {
			return fmt.Errorf("制品 %q 与 %q 使用了相同安装目标 %q", previous, artifact.Name, target)
		}
		targets[target] = artifact.Name
		if err := validateSource(artifact.Source); err != nil {
			return fmt.Errorf("%s.source 无效: %w", path, err)
		}
		if err := validateHelmValues(artifact.InstallOptions.HelmValues); err != nil {
			return fmt.Errorf("%s.installOptions.helmValues: %w", path, err)
		}
		if err := validateAnnotations(artifact.InstallOptions.Annotations); err != nil {
			return fmt.Errorf("%s.installOptions.annotations: %w", path, err)
		}
		if err := validateFailurePolicy(artifact.FailurePolicy); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}

	for _, artifact := range profile.Spec.Artifacts {
		for _, dependency := range artifact.DependsOn {
			if dependency == artifact.Name {
				return fmt.Errorf("制品 %q 不能依赖自身", artifact.Name)
			}
			if _, exists := byName[dependency]; !exists {
				return fmt.Errorf("制品 %q 引用了不存在的依赖 %q", artifact.Name, dependency)
			}
		}
	}
	if cycle := dependencyCycle(byName); len(cycle) > 0 {
		return fmt.Errorf("制品依赖存在环: %s", strings.Join(cycle, " -> "))
	}
	return nil
}

func validateHelmValues(values map[string]string) error {
	if len(values) > 100 {
		return errors.New("最多允许 100 个参数")
	}
	for key, value := range values {
		if strings.TrimSpace(key) == "" {
			return errors.New("参数名不能为空")
		}
		if len(key) > 253 || len(value) > 4096 {
			return fmt.Errorf("参数 %q 的名称或值过长", key)
		}
		if err := strvals.ParseInto(key+"="+value, map[string]interface{}{}); err != nil {
			return fmt.Errorf("参数 %q 无效: %w", key, err)
		}
	}
	return nil
}

func validateAnnotations(annotations map[string]string) error {
	if len(annotations) > 100 {
		return errors.New("最多允许 100 个注解")
	}
	for key, value := range annotations {
		if errs := validation.IsQualifiedName(key); len(errs) > 0 {
			return fmt.Errorf("注解名 %q 无效: %s", key, strings.Join(errs, ", "))
		}
		if key == bootstrapv1.AnnotationArtifactOwner {
			return fmt.Errorf("注解 %q 由 Bootstrap 内部维护，不能自定义", key)
		}
		if len(value) > 16*1024 {
			return fmt.Errorf("注解 %q 的值不能超过 16 KiB", key)
		}
	}
	return nil
}

func validateFailurePolicy(policy bootstrapv1.FailurePolicy) error {
	if policy != "" && policy != bootstrapv1.FailurePolicyContinue && policy != bootstrapv1.FailurePolicyStop {
		return fmt.Errorf("failurePolicy %q 无效", policy)
	}
	return nil
}

func validateSource(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "https" {
		return errors.New("当前仅允许 HTTPS 地址")
	}
	if u.Hostname() == "" {
		return errors.New("地址缺少主机名")
	}
	if u.User != nil {
		return errors.New("source 不能包含用户名或密码，请使用 Secret 引用")
	}
	for key := range u.Query() {
		normalized := strings.ToLower(key)
		if strings.Contains(normalized, "token") || strings.Contains(normalized, "password") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "auth") {
			return errors.New("source 不能在查询参数中包含凭据，请使用 Secret 引用")
		}
	}
	allowed := map[string]struct{}{
		"zpk.w7.cc":            {},
		"zpk.fan.b2.sz.w7.com": {},
	}
	for _, host := range strings.Split(os.Getenv("BOOTSTRAP_ALLOWED_SOURCE_HOSTS"), ",") {
		host = strings.TrimSpace(strings.ToLower(host))
		if host != "" {
			allowed[host] = struct{}{}
		}
	}
	if _, ok := allowed[strings.ToLower(u.Hostname())]; !ok {
		return fmt.Errorf("主机 %q 不在 BOOTSTRAP_ALLOWED_SOURCE_HOSTS 白名单中", u.Hostname())
	}
	return nil
}

func dependencyCycle(artifacts map[string]bootstrapv1.BootstrapArtifact) []string {
	state := make(map[string]uint8, len(artifacts))
	stack := make([]string, 0, len(artifacts))
	var visit func(string) []string
	visit = func(name string) []string {
		state[name] = 1
		stack = append(stack, name)
		for _, dependency := range artifacts[name].DependsOn {
			switch state[dependency] {
			case 0:
				if cycle := visit(dependency); len(cycle) > 0 {
					return cycle
				}
			case 1:
				for i, item := range stack {
					if item == dependency {
						return append(append([]string(nil), stack[i:]...), dependency)
					}
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[name] = 2
		return nil
	}
	names := make([]string, 0, len(artifacts))
	for name := range artifacts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if state[name] == 0 {
			if cycle := visit(name); len(cycle) > 0 {
				return cycle
			}
		}
	}
	return nil
}

func compareVersions(left, right string) int {
	leftSemver, rightSemver := normalizeSemver(left), normalizeSemver(right)
	if semver.IsValid(leftSemver) && semver.IsValid(rightSemver) {
		return semver.Compare(leftSemver, rightSemver)
	}
	return strings.Compare(left, right)
}

func normalizeSemver(version string) string {
	version = strings.TrimSpace(version)
	if version != "" && !strings.HasPrefix(version, "v") {
		return "v" + version
	}
	return version
}

func terminalPhase(phase bootstrapv1.BootstrapPhase) bool {
	switch phase {
	case bootstrapv1.BootstrapPhaseReady,
		bootstrapv1.BootstrapPhaseFailed:
		return true
	default:
		return false
	}
}

func setCondition(conditions *[]metav1.Condition, condition metav1.Condition) {
	apiMeta.SetStatusCondition(conditions, condition)
}
