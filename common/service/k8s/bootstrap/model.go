package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	installationv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrapinstallation/v1alpha1"
	"golang.org/x/mod/semver"
	"helm.sh/helm/v3/pkg/strvals"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	defaultMaxConcurrent int32 = 3
	defaultMaxRetries    int32 = 3
	bootstrapSlotScope         = "installations"
)

var defaultArtifactTimeout = 10 * time.Minute

type effectiveSettings struct {
	MaxConcurrent      int32
	MaxRetries         int32
	TimeoutPerArtifact time.Duration
}

func installationSettings(installation *installationv1.BootstrapInstallation) effectiveSettings {
	settings := effectiveSettings{MaxConcurrent: defaultMaxConcurrent, MaxRetries: defaultMaxRetries, TimeoutPerArtifact: defaultArtifactTimeout}
	strategy := installation.Spec.Strategy
	if strategy.MaxConcurrent > 0 {
		settings.MaxConcurrent = strategy.MaxConcurrent
	}
	if strategy.MaxRetries != nil {
		settings.MaxRetries = *strategy.MaxRetries
	}
	if strategy.TimeoutPerArtifact.Duration > 0 {
		settings.TimeoutPerArtifact = strategy.TimeoutPerArtifact.Duration
	}
	return settings
}

func effectiveArtifactType(value installationv1.ArtifactType) installationv1.ArtifactType {
	if value == "" {
		return installationv1.ArtifactTypeZPK
	}
	return value
}

func operationID(installation *installationv1.BootstrapInstallation) string {
	input := fmt.Sprintf("%s\x00%s\x00%s\x00%s", installation.UID, installation.Spec.Revision, installation.Spec.Artifact.Name, installation.Spec.Artifact.Version)
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:16])
}

func validateInstallation(installation *installationv1.BootstrapInstallation) error {
	if installation.Spec.Revision == "" {
		return errors.New("spec.revision 不能为空")
	}
	if errs := validation.IsValidLabelValue(installation.Name); len(errs) > 0 {
		return fmt.Errorf("Installation 名称不能作为标签值: %s", strings.Join(errs, ", "))
	}
	strategy := installation.Spec.Strategy
	settings := installationSettings(installation)
	if strategy.MaxConcurrent < 0 || (strategy.MaxRetries != nil && *strategy.MaxRetries < 0) || strategy.TimeoutPerArtifact.Duration < 0 {
		return errors.New("strategy 中的数值不能为负数")
	}
	if settings.MaxConcurrent > 50 {
		return errors.New("strategy.maxConcurrent 不能大于 50")
	}
	artifact := installation.Spec.Artifact
	if artifact.Name == "" || artifact.Identifie == "" || artifact.Source == "" || installation.Spec.Target.ReleaseName == "" || installation.Spec.Target.Namespace == "" {
		return errors.New("spec.artifact 的 name、identifie、source 以及 spec.target 的 releaseName、namespace 均为必填项")
	}
	if effectiveArtifactType(artifact.Type) != installationv1.ArtifactTypeZPK {
		return fmt.Errorf("spec.artifact.type %q 当前不支持，仅 ZPK 执行器已启用", artifact.Type)
	}
	if errs := validation.IsDNS1123Label(artifact.Name); len(errs) > 0 {
		return fmt.Errorf("spec.artifact.name 无效: %s", strings.Join(errs, ", "))
	}
	if errs := validation.IsDNS1123Label(installation.Spec.Target.Namespace); len(errs) > 0 {
		return fmt.Errorf("spec.target.namespace 无效: %s", strings.Join(errs, ", "))
	}
	if errs := validation.IsDNS1123Subdomain(installation.Spec.Target.ReleaseName); len(errs) > 0 || len(installation.Spec.Target.ReleaseName) > 53 {
		return fmt.Errorf("spec.target.releaseName 必须是最长 53 字符的 DNS 子域名: %s", strings.Join(errs, ", "))
	}
	if errs := validation.IsValidLabelValue(normalizeIdentifie(artifact.Identifie)); len(errs) > 0 {
		return fmt.Errorf("spec.artifact.identifie 无效: %s", strings.Join(errs, ", "))
	}
	if err := validateSource(artifact.Source); err != nil {
		return fmt.Errorf("spec.artifact.source 无效: %w", err)
	}
	if err := validateHelmValues(installation.Spec.InstallOptions.HelmValues); err != nil {
		return fmt.Errorf("spec.installOptions.helmValues: %w", err)
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

func validateSource(source string) error {
	parsed, err := url.Parse(source)
	if err != nil {
		return err
	}
	if parsed.Scheme != "https" {
		return errors.New("仅支持 HTTPS ZPK source")
	}
	if parsed.User != nil {
		return errors.New("source 不能包含用户名或密码")
	}
	for key := range parsed.Query() {
		normalized := strings.ToLower(key)
		if strings.Contains(normalized, "token") || strings.Contains(normalized, "password") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "auth") {
			return errors.New("source 不能在查询参数中包含凭据")
		}
	}
	if parsed.Hostname() == "" {
		return errors.New("source 缺少主机名")
	}
	allowed := map[string]struct{}{"zpk.w7.cc": {}, "zpk.fan.b2.sz.w7.com": {}}
	for _, host := range strings.Split(os.Getenv("BOOTSTRAP_ALLOWED_SOURCE_HOSTS"), ",") {
		if host = strings.ToLower(strings.TrimSpace(host)); host != "" {
			allowed[host] = struct{}{}
		}
	}
	if _, ok := allowed[strings.ToLower(parsed.Hostname())]; !ok {
		return fmt.Errorf("主机 %q 不在允许列表", parsed.Hostname())
	}
	return nil
}

func terminalPhase(phase installationv1.BootstrapPhase) bool {
	return phase == installationv1.BootstrapPhaseReady || phase == installationv1.BootstrapPhaseFailed
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
