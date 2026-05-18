package pid

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

/**
前端传递apiversion 和 kind 字段，以及name 字段，然后根据这个获取对应deployment statefulset daemonset 然后根据获取到的资源 读取当前挂载的文件列表
文件列表返回 json 格式的文件列表 这个文件对应的哪个secret 或者 configmap的key 等信息, 如有必要获取configmap 或者 secret 的内容
*/

type MountFiles struct {
	*k8s.Sdk
}

type MountFilesParam struct {
	Namespace      string `form:"namespace"`
	APIVersion     string `form:"apiVersion" binding:"required"`
	Kind           string `form:"kind" binding:"required"`
	Name           string `form:"name" binding:"required"`
	IncludeContent bool   `form:"includeContent"`
}

type MountFilesResult struct {
	Namespace  string                 `json:"namespace"`
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Name       string                 `json:"name"`
	Mounts     []MountFileDescription `json:"mounts"`
}

type MountFileDescription struct {
	ContainerName string           `json:"containerName"`
	ContainerType string           `json:"containerType,omitempty"`
	VolumeName    string           `json:"volumeName"`
	MountPath     string           `json:"mountPath"`
	SubPath       string           `json:"subPath,omitempty"`
	ReadOnly      bool             `json:"readOnly"`
	SourceType    string           `json:"sourceType"`
	SourceName    string           `json:"sourceName,omitempty"`
	Files         []MountFileEntry `json:"files"`
}

type MountFileEntry struct {
	Path          string `json:"path"`
	RelativePath  string `json:"relativePath,omitempty"`
	SourceType    string `json:"sourceType"`
	SourceName    string `json:"sourceName,omitempty"`
	Key           string `json:"key,omitempty"`
	Optional      bool   `json:"optional,omitempty"`
	Content       string `json:"content,omitempty"`
	ContentBase64 string `json:"contentBase64,omitempty"`
	Binary        bool   `json:"binary,omitempty"`
}

func NewMountFiles(sdk *k8s.Sdk) *MountFiles {
	return &MountFiles{
		Sdk: sdk,
	}
}

func NewMountFilesByToken(token string) (*MountFiles, error) {
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		return nil, err
	}
	return NewMountFiles(sdk), nil
}

func (m *MountFiles) Handle(param MountFilesParam) (*MountFilesResult, error) {
	namespace := param.Namespace
	if namespace == "" {
		namespace = m.GetNamespace()
	}

	obj, err := m.GetK8sRawObject(param.Name, param.APIVersion, param.Kind, namespace)
	if err != nil {
		return nil, err
	}

	if obj.GetNamespace() != "" {
		namespace = obj.GetNamespace()
	}

	podSpec, err := workloadPodSpec(obj)
	if err != nil {
		return nil, err
	}

	volumeMap := make(map[string]corev1.Volume, len(podSpec.Volumes))
	for _, volume := range podSpec.Volumes {
		volumeMap[volume.Name] = volume
	}

	result := &MountFilesResult{
		Namespace:  namespace,
		APIVersion: param.APIVersion,
		Kind:       param.Kind,
		Name:       obj.GetName(),
		Mounts:     make([]MountFileDescription, 0),
	}

	result.Mounts = append(result.Mounts, m.describeContainerMounts(namespace, "container", podSpec.Containers, volumeMap, param.IncludeContent)...)
	result.Mounts = append(result.Mounts, m.describeContainerMounts(namespace, "initContainer", podSpec.InitContainers, volumeMap, param.IncludeContent)...)

	return result, nil
}

func workloadPodSpec(obj *unstructured.Unstructured) (*corev1.PodSpec, error) {
	podSpecMap, found, err := unstructured.NestedMap(obj.Object, "spec", "template", "spec")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s/%s does not contain spec.template.spec", obj.GetKind(), obj.GetName())
	}
	podSpec := &corev1.PodSpec{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podSpecMap, podSpec); err != nil {
		return nil, err
	}
	return podSpec, nil
}

func (m *MountFiles) describeContainerMounts(namespace, containerType string, containers []corev1.Container, volumeMap map[string]corev1.Volume, includeContent bool) []MountFileDescription {
	result := make([]MountFileDescription, 0)
	for _, container := range containers {
		for _, volumeMount := range container.VolumeMounts {
			volume, ok := volumeMap[volumeMount.Name]
			if !ok {
				continue
			}

			files := m.describeVolumeFiles(context.TODO(), namespace, volume, volumeMount, includeContent)
			if len(files) == 0 {
				continue
			}

			desc := MountFileDescription{
				ContainerName: container.Name,
				ContainerType: containerType,
				VolumeName:    volume.Name,
				MountPath:     volumeMount.MountPath,
				SubPath:       volumeMount.SubPath,
				ReadOnly:      volumeMount.ReadOnly,
				Files:         files,
			}
			desc.SourceType = files[0].SourceType
			desc.SourceName = files[0].SourceName
			result = append(result, desc)
		}
	}
	return result
}

func (m *MountFiles) describeVolumeFiles(ctx context.Context, namespace string, volume corev1.Volume, volumeMount corev1.VolumeMount, includeContent bool) []MountFileEntry {
	switch {
	case volume.ConfigMap != nil:
		return m.describeConfigMapVolume(ctx, namespace, volume.ConfigMap, volumeMount, includeContent)
	case volume.Secret != nil:
		return m.describeSecretVolume(ctx, namespace, volume.Secret, volumeMount, includeContent)
	case volume.Projected != nil:
		return m.describeProjectedVolume(ctx, namespace, volume.Projected, volumeMount, includeContent)
	default:
		return nil
	}
}

func (m *MountFiles) describeConfigMapVolume(ctx context.Context, namespace string, source *corev1.ConfigMapVolumeSource, volumeMount corev1.VolumeMount, includeContent bool) []MountFileEntry {
	if source.Name == "" {
		return nil
	}

	cm, err := m.ClientSet.CoreV1().ConfigMaps(namespace).Get(ctx, source.Name, metav1.GetOptions{})
	if err != nil {
		return []MountFileEntry{{
			Path:       mountTargetPath(volumeMount, ""),
			SourceType: "configMap",
			SourceName: source.Name,
		}}
	}

	entries := make([]MountFileEntry, 0)
	if len(source.Items) > 0 {
		for _, item := range source.Items {
			entry := MountFileEntry{
				Path:         mountTargetPath(volumeMount, item.Path),
				RelativePath: item.Path,
				SourceType:   "configMap",
				SourceName:   source.Name,
				Key:          item.Key,
				Optional:     source.Optional != nil && *source.Optional,
			}
			if val, ok := cm.Data[item.Key]; ok && includeContent {
				entry.Content = val
			}
			if val, ok := cm.BinaryData[item.Key]; ok && includeContent {
				entry.ContentBase64 = base64.StdEncoding.EncodeToString(val)
				entry.Binary = true
			}
			entries = append(entries, entry)
		}
		return filterBySubPath(entries, volumeMount)
	}

	for key, val := range cm.Data {
		entries = append(entries, MountFileEntry{
			Path:         mountTargetPath(volumeMount, key),
			RelativePath: key,
			SourceType:   "configMap",
			SourceName:   source.Name,
			Key:          key,
			Optional:     source.Optional != nil && *source.Optional,
			Content:      contentIf(includeContent, val),
		})
	}
	for key, val := range cm.BinaryData {
		entry := MountFileEntry{
			Path:         mountTargetPath(volumeMount, key),
			RelativePath: key,
			SourceType:   "configMap",
			SourceName:   source.Name,
			Key:          key,
			Optional:     source.Optional != nil && *source.Optional,
		}
		if includeContent {
			entry.ContentBase64 = base64.StdEncoding.EncodeToString(val)
			entry.Binary = true
		}
		entries = append(entries, entry)
	}
	return filterBySubPath(entries, volumeMount)
}

func (m *MountFiles) describeSecretVolume(ctx context.Context, namespace string, source *corev1.SecretVolumeSource, volumeMount corev1.VolumeMount, includeContent bool) []MountFileEntry {
	if source.SecretName == "" {
		return nil
	}

	secret, err := m.ClientSet.CoreV1().Secrets(namespace).Get(ctx, source.SecretName, metav1.GetOptions{})
	if err != nil {
		return []MountFileEntry{{
			Path:       mountTargetPath(volumeMount, ""),
			SourceType: "secret",
			SourceName: source.SecretName,
		}}
	}

	entries := make([]MountFileEntry, 0)
	if len(source.Items) > 0 {
		for _, item := range source.Items {
			entry := MountFileEntry{
				Path:         mountTargetPath(volumeMount, item.Path),
				RelativePath: item.Path,
				SourceType:   "secret",
				SourceName:   source.SecretName,
				Key:          item.Key,
				Optional:     source.Optional != nil && *source.Optional,
			}
			if val, ok := secret.Data[item.Key]; ok && includeContent {
				fillSecretContent(&entry, val)
			}
			entries = append(entries, entry)
		}
		return filterBySubPath(entries, volumeMount)
	}

	for key, val := range secret.Data {
		entry := MountFileEntry{
			Path:         mountTargetPath(volumeMount, key),
			RelativePath: key,
			SourceType:   "secret",
			SourceName:   source.SecretName,
			Key:          key,
			Optional:     source.Optional != nil && *source.Optional,
		}
		if includeContent {
			fillSecretContent(&entry, val)
		}
		entries = append(entries, entry)
	}
	return filterBySubPath(entries, volumeMount)
}

func (m *MountFiles) describeProjectedVolume(ctx context.Context, namespace string, source *corev1.ProjectedVolumeSource, volumeMount corev1.VolumeMount, includeContent bool) []MountFileEntry {
	result := make([]MountFileEntry, 0)
	for _, item := range source.Sources {
		switch {
		case item.ConfigMap != nil:
			result = append(result, m.describeConfigMapProjection(ctx, namespace, item.ConfigMap, volumeMount, includeContent)...)
		case item.Secret != nil:
			result = append(result, m.describeSecretProjection(ctx, namespace, item.Secret, volumeMount, includeContent)...)
		case item.ServiceAccountToken != nil:
			entry := MountFileEntry{
				Path:         mountTargetPath(volumeMount, item.ServiceAccountToken.Path),
				RelativePath: item.ServiceAccountToken.Path,
				SourceType:   "serviceAccountToken",
			}
			result = append(result, entry)
		}
	}
	return filterBySubPath(result, volumeMount)
}

func (m *MountFiles) describeConfigMapProjection(ctx context.Context, namespace string, source *corev1.ConfigMapProjection, volumeMount corev1.VolumeMount, includeContent bool) []MountFileEntry {
	if source == nil || source.Name == "" {
		return nil
	}
	cm, err := m.ClientSet.CoreV1().ConfigMaps(namespace).Get(ctx, source.Name, metav1.GetOptions{})
	if err != nil {
		return []MountFileEntry{{
			Path:       mountTargetPath(volumeMount, ""),
			SourceType: "configMap",
			SourceName: source.Name,
		}}
	}

	result := make([]MountFileEntry, 0)
	if len(source.Items) > 0 {
		for _, projectionItem := range source.Items {
			entry := MountFileEntry{
				Path:         mountTargetPath(volumeMount, projectionItem.Path),
				RelativePath: projectionItem.Path,
				SourceType:   "configMap",
				SourceName:   source.Name,
				Key:          projectionItem.Key,
				Optional:     source.Optional != nil && *source.Optional,
			}
			if val, ok := cm.Data[projectionItem.Key]; ok && includeContent {
				entry.Content = val
			}
			if val, ok := cm.BinaryData[projectionItem.Key]; ok && includeContent {
				entry.ContentBase64 = base64.StdEncoding.EncodeToString(val)
				entry.Binary = true
			}
			result = append(result, entry)
		}
		return result
	}

	for key, val := range cm.Data {
		result = append(result, MountFileEntry{
			Path:         mountTargetPath(volumeMount, key),
			RelativePath: key,
			SourceType:   "configMap",
			SourceName:   source.Name,
			Key:          key,
			Optional:     source.Optional != nil && *source.Optional,
			Content:      contentIf(includeContent, val),
		})
	}
	for key, val := range cm.BinaryData {
		entry := MountFileEntry{
			Path:         mountTargetPath(volumeMount, key),
			RelativePath: key,
			SourceType:   "configMap",
			SourceName:   source.Name,
			Key:          key,
			Optional:     source.Optional != nil && *source.Optional,
		}
		if includeContent {
			entry.ContentBase64 = base64.StdEncoding.EncodeToString(val)
			entry.Binary = true
		}
		result = append(result, entry)
	}
	return result
}

func (m *MountFiles) describeSecretProjection(ctx context.Context, namespace string, source *corev1.SecretProjection, volumeMount corev1.VolumeMount, includeContent bool) []MountFileEntry {
	if source == nil || source.Name == "" {
		return nil
	}
	secret, err := m.ClientSet.CoreV1().Secrets(namespace).Get(ctx, source.Name, metav1.GetOptions{})
	if err != nil {
		return []MountFileEntry{{
			Path:       mountTargetPath(volumeMount, ""),
			SourceType: "secret",
			SourceName: source.Name,
		}}
	}

	result := make([]MountFileEntry, 0)
	if len(source.Items) > 0 {
		for _, projectionItem := range source.Items {
			entry := MountFileEntry{
				Path:         mountTargetPath(volumeMount, projectionItem.Path),
				RelativePath: projectionItem.Path,
				SourceType:   "secret",
				SourceName:   source.Name,
				Key:          projectionItem.Key,
				Optional:     source.Optional != nil && *source.Optional,
			}
			if val, ok := secret.Data[projectionItem.Key]; ok && includeContent {
				fillSecretContent(&entry, val)
			}
			result = append(result, entry)
		}
		return result
	}

	for key, val := range secret.Data {
		entry := MountFileEntry{
			Path:         mountTargetPath(volumeMount, key),
			RelativePath: key,
			SourceType:   "secret",
			SourceName:   source.Name,
			Key:          key,
			Optional:     source.Optional != nil && *source.Optional,
		}
		if includeContent {
			fillSecretContent(&entry, val)
		}
		result = append(result, entry)
	}
	return result
}

func mountTargetPath(volumeMount corev1.VolumeMount, relativePath string) string {
	if volumeMount.SubPath != "" {
		return volumeMount.MountPath
	}
	if relativePath == "" {
		return volumeMount.MountPath
	}
	return path.Join(volumeMount.MountPath, relativePath)
}

func filterBySubPath(entries []MountFileEntry, volumeMount corev1.VolumeMount) []MountFileEntry {
	if volumeMount.SubPath == "" {
		return entries
	}
	result := make([]MountFileEntry, 0)
	for _, entry := range entries {
		if entry.RelativePath == volumeMount.SubPath {
			entry.Path = volumeMount.MountPath
			result = append(result, entry)
		}
	}
	if len(result) > 0 {
		return result
	}
	return entries
}

func fillSecretContent(entry *MountFileEntry, data []byte) {
	if utf8.Valid(data) && !strings.ContainsRune(string(data), '\x00') {
		entry.Content = string(data)
		return
	}
	entry.ContentBase64 = base64.StdEncoding.EncodeToString(data)
	entry.Binary = true
}

func contentIf(include bool, content string) string {
	if include {
		return content
	}
	return ""
}
