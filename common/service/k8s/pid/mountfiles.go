package pid

import (
	"context"
	"encoding/base64"
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/w7panel/w7panel/common/helper"
	"github.com/w7panel/w7panel/common/service/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

/**
1. 前端传递apiversion 和 kind 字段，以及name 字段，然后根据这个获取对应deployment statefulset daemonset 然后根据获取到的资源 读取当前挂载的文件列表
文件列表返回 json 格式的文件列表 这个文件对应的哪个secret 或者 configmap的key 等信息, 如有必要获取configmap 或者 secret 的内容
2. common/service/k8s/pid/mountfiles.go 添加一个方法 参数是绝对路径和文件内容 根据获取映射关系 直接修改对应的secret或者configmap
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

type UpdateMountFileParam struct {
	Namespace     string `form:"namespace"`
	APIVersion    string `form:"apiVersion" binding:"required"`
	Kind          string `form:"kind" binding:"required"`
	Name          string `form:"name" binding:"required"`
	Path          string `form:"path" binding:"required"`
	ContainerName string `form:"containerName"`
	Action        string `form:"action"`
	Content       string `form:"content"`
	Mode          string `form:"mode"`
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
	Mode          *int32 `json:"mode,omitempty"`
	ModeOctal     string `json:"modeOctal,omitempty"`
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

func (m *MountFiles) UpdateFileContent(param UpdateMountFileParam) error {
	action := strings.ToLower(strings.TrimSpace(param.Action))
	if action == "" {
		action = "update"
	}

	switch action {
	case "create":
		return m.createFile(param)
	case "update":
		return m.updateFileContent(param)
	case "delete", "remove":
		return m.deleteFile(param)
	case "chmod":
		return m.chmodFile(param)
	default:
		return fmt.Errorf("unsupported action: %s", param.Action)
	}
}

type createMountTarget struct {
	ContainerName string
	ContainerType string
	MountPath     string
}

func (m *MountFiles) createFile(param UpdateMountFileParam) error {
	namespace := param.Namespace
	if namespace == "" {
		namespace = m.GetNamespace()
	}

	obj, err := m.GetK8sRawObject(param.Name, param.APIVersion, param.Kind, namespace)
	if err != nil {
		return err
	}
	if obj.GetNamespace() != "" {
		namespace = obj.GetNamespace()
	}

	podSpec, err := workloadPodSpec(obj)
	if err != nil {
		return err
	}

	target, err := findCreateMountTargetFromPodSpec(podSpec, param.Path, param.ContainerName)
	if err != nil {
		return err
	}

	configMapName := buildMountFileConfigMapName(obj.GetName(), path.Base(param.Path))
	volumeName := buildMountFileVolumeName(path.Base(param.Path))
	fileKey := path.Base(param.Path)
	fileMode := int32(corev1.ConfigMapVolumeSourceDefaultMode)

	if param.Mode != "" {
		fileMode, err = parseFileMode(param.Mode)
		if err != nil {
			return err
		}
	}

	ctx := context.TODO()
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"w7.cc/mountfile": "true",
				"w7.cc/workload":  obj.GetName(),
			},
		},
		Data: map[string]string{
			fileKey: param.Content,
		},
	}
	if _, err := m.ClientSet.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
		return err
	}

	err = attachCreatedFileToPodSpec(podSpec, *target, volumeName, configMapName, fileKey, param.Path, fileMode)
	if err != nil {
		return err
	}

	podSpecMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(podSpec)
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedMap(obj.Object, podSpecMap, "spec", "template", "spec"); err != nil {
		return err
	}
	objBytes, err := obj.MarshalJSON()
	if err != nil {
		return err
	}
	_, err = m.ApplyJson(objBytes, k8s.ApplyOptions{Namespace: namespace})
	return err
}

func (m *MountFiles) updateFileContent(param UpdateMountFileParam) error {
	result, err := m.Handle(MountFilesParam{
		Namespace:      param.Namespace,
		APIVersion:     param.APIVersion,
		Kind:           param.Kind,
		Name:           param.Name,
		IncludeContent: false,
	})
	if err != nil {
		return err
	}

	target, err := findMountFileTarget(result, param.Path)
	if err != nil {
		return err
	}

	ctx := context.TODO()
	namespace := result.Namespace

	switch target.SourceType {
	case "configMap":
		cm, err := m.ClientSet.CoreV1().ConfigMaps(namespace).Get(ctx, target.SourceName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if cm.Data == nil {
			cm.Data = make(map[string]string)
		}
		if cm.BinaryData != nil {
			if _, ok := cm.BinaryData[target.Key]; ok {
				cm.BinaryData[target.Key] = []byte(param.Content)
				_, err = m.ClientSet.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
				return err
			}
		}
		cm.Data[target.Key] = param.Content
		_, err = m.ClientSet.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
		return err
	case "secret":
		secret, err := m.ClientSet.CoreV1().Secrets(namespace).Get(ctx, target.SourceName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		secret.Data[target.Key] = []byte(param.Content)
		_, err = m.ClientSet.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
		return err
	default:
		return fmt.Errorf("path %s maps to unsupported source type %s", param.Path, target.SourceType)
	}
}

func (m *MountFiles) deleteFile(param UpdateMountFileParam) error {
	result, err := m.Handle(MountFilesParam{
		Namespace:      param.Namespace,
		APIVersion:     param.APIVersion,
		Kind:           param.Kind,
		Name:           param.Name,
		IncludeContent: false,
	})
	if err != nil {
		return err
	}

	target, err := findMountFileTarget(result, param.Path)
	if err != nil {
		return err
	}

	ctx := context.TODO()
	namespace := result.Namespace
	obj, err := m.GetK8sRawObject(param.Name, param.APIVersion, param.Kind, namespace)
	if err != nil {
		return err
	}
	if obj.GetNamespace() != "" {
		namespace = obj.GetNamespace()
	}

	podSpec, err := workloadPodSpec(obj)
	if err != nil {
		return err
	}

	switch target.SourceType {
	case "configMap":
		cm, err := m.ClientSet.CoreV1().ConfigMaps(namespace).Get(ctx, target.SourceName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if configMapEntryCount(cm) <= 1 {
			removeConfigMapVolumesFromPodSpec(podSpec, target.SourceName)
			if err := m.applyWorkloadPodSpec(obj, podSpec, namespace); err != nil {
				return err
			}
			return m.ClientSet.CoreV1().ConfigMaps(namespace).Delete(ctx, target.SourceName, metav1.DeleteOptions{})
		}
		removeMountedFileReferencesFromPodSpec(podSpec, target)
		if err := m.applyWorkloadPodSpec(obj, podSpec, namespace); err != nil {
			return err
		}
		delete(cm.Data, target.Key)
		delete(cm.BinaryData, target.Key)
		_, err = m.ClientSet.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
		return err
	case "secret":
		secret, err := m.ClientSet.CoreV1().Secrets(namespace).Get(ctx, target.SourceName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		removeMountedFileReferencesFromPodSpec(podSpec, target)
		if err := m.applyWorkloadPodSpec(obj, podSpec, namespace); err != nil {
			return err
		}
		delete(secret.Data, target.Key)
		_, err = m.ClientSet.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
		return err
	default:
		return fmt.Errorf("path %s maps to unsupported source type %s", param.Path, target.SourceType)
	}
}

func (m *MountFiles) applyWorkloadPodSpec(obj *unstructured.Unstructured, podSpec *corev1.PodSpec, namespace string) error {
	podSpecMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(podSpec)
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedMap(obj.Object, podSpecMap, "spec", "template", "spec"); err != nil {
		return err
	}

	objBytes, err := obj.MarshalJSON()
	if err != nil {
		return err
	}
	_, err = m.ApplyJson(objBytes, k8s.ApplyOptions{Namespace: namespace})
	return err
}

func (m *MountFiles) chmodFile(param UpdateMountFileParam) error {
	mode, err := parseFileMode(param.Mode)
	if err != nil {
		return err
	}

	namespace := param.Namespace
	if namespace == "" {
		namespace = m.GetNamespace()
	}

	obj, err := m.GetK8sRawObject(param.Name, param.APIVersion, param.Kind, namespace)
	if err != nil {
		return err
	}
	if obj.GetNamespace() != "" {
		namespace = obj.GetNamespace()
	}

	podSpec, err := workloadPodSpec(obj)
	if err != nil {
		return err
	}

	if !applyModeToMountedFile(podSpec, param.Path, mode) {
		return fmt.Errorf("path %s not found in mounted files", param.Path)
	}

	podSpecMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(podSpec)
	if err != nil {
		return err
	}
	if err := unstructured.SetNestedMap(obj.Object, podSpecMap, "spec", "template", "spec"); err != nil {
		return err
	}

	objBytes, err := obj.MarshalJSON()
	if err != nil {
		return err
	}
	_, err = m.ApplyJson(objBytes, k8s.ApplyOptions{Namespace: namespace})
	return err
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

func findMountFileTarget(result *MountFilesResult, absolutePath string) (*MountFileEntry, error) {
	if absolutePath == "" || !path.IsAbs(absolutePath) {
		return nil, fmt.Errorf("path must be absolute: %s", absolutePath)
	}

	var target *MountFileEntry
	for _, mount := range result.Mounts {
		for _, file := range mount.Files {
			if file.Path != absolutePath {
				continue
			}
			if file.SourceName == "" || file.Key == "" {
				return nil, fmt.Errorf("path %s does not map to a writable secret/configmap key", absolutePath)
			}
			if target == nil {
				entry := file
				target = &entry
				continue
			}
			if target.SourceType == file.SourceType && target.SourceName == file.SourceName && target.Key == file.Key {
				continue
			}
			return nil, fmt.Errorf("path %s matches multiple mount targets", absolutePath)
		}
	}

	if target == nil {
		return nil, fmt.Errorf("path %s not found in mounted files", absolutePath)
	}
	return target, nil
}

func findCreateMountTarget(result *MountFilesResult, absolutePath string) (*createMountTarget, error) {
	if absolutePath == "" || !path.IsAbs(absolutePath) {
		return nil, fmt.Errorf("必须是绝对路径: %s", absolutePath)
	}

	for _, mount := range result.Mounts {
		if hasMountedFile(mount.Files, absolutePath) {
			return nil, fmt.Errorf("路径 %s 已挂载", absolutePath)
		}
	}

	var target *createMountTarget
	longestMountPath := -1
	for _, mount := range result.Mounts {
		if mount.ReadOnly {
			continue
		}
		if !matchesMountRootByPath(mount.MountPath, absolutePath) {
			continue
		}
		if absolutePath == mount.MountPath {
			continue
		}
		candidate := &createMountTarget{
			ContainerName: mount.ContainerName,
			ContainerType: mount.ContainerType,
			MountPath:     mount.MountPath,
		}
		mountLen := len(strings.TrimRight(mount.MountPath, "/"))
		if target == nil || mountLen > longestMountPath {
			target = candidate
			longestMountPath = mountLen
			continue
		}
		if mountLen < longestMountPath {
			continue
		}
		if target.ContainerName == candidate.ContainerName && target.ContainerType == candidate.ContainerType && target.MountPath == candidate.MountPath {
			continue
		}
		return nil, fmt.Errorf("path %s matches multiple mount targets", absolutePath)
	}

	if target == nil {
		return nil, fmt.Errorf("path %s does not map to a creatable mount path", absolutePath)
	}
	return target, nil
}

func findCreateMountTargetFromPodSpec(podSpec *corev1.PodSpec, absolutePath, containerName string) (*createMountTarget, error) {
	if absolutePath == "" || !path.IsAbs(absolutePath) {
		return nil, fmt.Errorf("path must be absolute: %s", absolutePath)
	}

	target, err := findCreateMountTargetInContainers(podSpec.Containers, "container", absolutePath, containerName)
	if err != nil || target != nil {
		return target, err
	}

	initTarget, initErr := findCreateMountTargetInContainers(podSpec.InitContainers, "initContainer", absolutePath, containerName)
	if initErr != nil || initTarget != nil {
		return initTarget, initErr
	}

	return findStandaloneCreateMountTarget(podSpec, absolutePath, containerName)
}

func findCreateMountTargetInContainers(containers []corev1.Container, containerType, absolutePath, containerName string) (*createMountTarget, error) {
	var target *createMountTarget
	longestMountPath := -1

	for _, container := range containers {
		if containerName != "" && container.Name != containerName {
			continue
		}
		for _, mount := range container.VolumeMounts {
			if mount.ReadOnly {
				continue
			}
			if mount.MountPath == absolutePath {
				return nil, fmt.Errorf("path %s already exists in mounted files", absolutePath)
			}
			if !matchesMountRoot(mount, absolutePath) || absolutePath == mount.MountPath {
				continue
			}

			candidate := &createMountTarget{
				ContainerName: container.Name,
				ContainerType: containerType,
				MountPath:     mount.MountPath,
			}
			mountLen := len(strings.TrimRight(mount.MountPath, "/"))
			if target == nil || mountLen > longestMountPath {
				target = candidate
				longestMountPath = mountLen
				continue
			}
			if mountLen < longestMountPath {
				continue
			}
			if target.ContainerName == candidate.ContainerName && target.ContainerType == candidate.ContainerType && target.MountPath == candidate.MountPath {
				continue
			}
			return nil, fmt.Errorf("path %s matches multiple mount targets", absolutePath)
		}
	}

	if target == nil {
		return nil, nil
	}
	return target, nil
}

func findStandaloneCreateMountTarget(podSpec *corev1.PodSpec, absolutePath, containerName string) (*createMountTarget, error) {
	if hasConflictingMountPath(podSpec.Containers, absolutePath) || hasConflictingMountPath(podSpec.InitContainers, absolutePath) {
		return nil, fmt.Errorf("path %s conflicts with an existing mount path", absolutePath)
	}

	if containerName != "" {
		if container := findContainerByName(podSpec.Containers, containerName); container != nil {
			return &createMountTarget{
				ContainerName: container.Name,
				ContainerType: "container",
				MountPath:     absolutePath,
			}, nil
		}
		if container := findContainerByName(podSpec.InitContainers, containerName); container != nil {
			return &createMountTarget{
				ContainerName: container.Name,
				ContainerType: "initContainer",
				MountPath:     absolutePath,
			}, nil
		}
		return nil, fmt.Errorf("container %s not found", containerName)
	}

	if len(podSpec.Containers) != 1 {
		return nil, fmt.Errorf("path %s is outside existing mount paths and workload must have exactly one container or specify containerName", absolutePath)
	}

	return &createMountTarget{
		ContainerName: podSpec.Containers[0].Name,
		ContainerType: "container",
		MountPath:     absolutePath,
	}, nil
}

func findContainerByName(containers []corev1.Container, containerName string) *corev1.Container {
	for i := range containers {
		if containers[i].Name == containerName {
			return &containers[i]
		}
	}
	return nil
}

func hasConflictingMountPath(containers []corev1.Container, absolutePath string) bool {
	for _, container := range containers {
		for _, mount := range container.VolumeMounts {
			if mount.MountPath == absolutePath {
				return true
			}
			if mount.SubPath != "" {
				prefix := strings.TrimRight(mount.MountPath, "/") + "/"
				if strings.HasPrefix(absolutePath, prefix) {
					return true
				}
				continue
			}
			if matchesMountRootByPath(mount.MountPath, absolutePath) {
				return true
			}
		}
	}
	return false
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
			entry.Mode, entry.ModeOctal = effectiveFileMode(item.Mode, source.DefaultMode, corev1.ConfigMapVolumeSourceDefaultMode)
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
			Mode:         modePtr(corev1.ConfigMapVolumeSourceDefaultMode, source.DefaultMode),
			ModeOctal:    formatFileMode(valueOrDefault(source.DefaultMode, corev1.ConfigMapVolumeSourceDefaultMode)),
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
		entry.Mode, entry.ModeOctal = effectiveFileMode(nil, source.DefaultMode, corev1.ConfigMapVolumeSourceDefaultMode)
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
			entry.Mode, entry.ModeOctal = effectiveFileMode(item.Mode, source.DefaultMode, corev1.SecretVolumeSourceDefaultMode)
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
		entry.Mode, entry.ModeOctal = effectiveFileMode(nil, source.DefaultMode, corev1.SecretVolumeSourceDefaultMode)
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
			result = append(result, m.describeConfigMapProjection(ctx, namespace, item.ConfigMap, volumeMount, includeContent, source.DefaultMode)...)
		case item.Secret != nil:
			result = append(result, m.describeSecretProjection(ctx, namespace, item.Secret, volumeMount, includeContent, source.DefaultMode)...)
		case item.ServiceAccountToken != nil:
			entry := MountFileEntry{
				Path:         mountTargetPath(volumeMount, item.ServiceAccountToken.Path),
				RelativePath: item.ServiceAccountToken.Path,
				SourceType:   "serviceAccountToken",
			}
			entry.Mode, entry.ModeOctal = effectiveFileMode(nil, source.DefaultMode, corev1.ProjectedVolumeSourceDefaultMode)
			result = append(result, entry)
		}
	}
	return filterBySubPath(result, volumeMount)
}

func (m *MountFiles) describeConfigMapProjection(ctx context.Context, namespace string, source *corev1.ConfigMapProjection, volumeMount corev1.VolumeMount, includeContent bool, projectedDefaultMode *int32) []MountFileEntry {
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
			entry.Mode, entry.ModeOctal = effectiveFileMode(projectionItem.Mode, projectedDefaultMode, corev1.ProjectedVolumeSourceDefaultMode)
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
			Mode:         modePtr(valueOrDefault(projectedDefaultMode, corev1.ProjectedVolumeSourceDefaultMode), nil),
			ModeOctal:    formatFileMode(valueOrDefault(projectedDefaultMode, corev1.ProjectedVolumeSourceDefaultMode)),
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
		entry.Mode, entry.ModeOctal = effectiveFileMode(nil, projectedDefaultMode, corev1.ProjectedVolumeSourceDefaultMode)
		if includeContent {
			entry.ContentBase64 = base64.StdEncoding.EncodeToString(val)
			entry.Binary = true
		}
		result = append(result, entry)
	}
	return result
}

func (m *MountFiles) describeSecretProjection(ctx context.Context, namespace string, source *corev1.SecretProjection, volumeMount corev1.VolumeMount, includeContent bool, projectedDefaultMode *int32) []MountFileEntry {
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
			entry.Mode, entry.ModeOctal = effectiveFileMode(projectionItem.Mode, projectedDefaultMode, corev1.ProjectedVolumeSourceDefaultMode)
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
		entry.Mode, entry.ModeOctal = effectiveFileMode(nil, projectedDefaultMode, corev1.ProjectedVolumeSourceDefaultMode)
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

func effectiveFileMode(itemMode, defaultMode *int32, fallback int32) (*int32, string) {
	mode := valueOrDefault(itemMode, valueOrDefault(defaultMode, fallback))
	return modePtr(mode, nil), formatFileMode(mode)
}

func valueOrDefault(mode *int32, fallback int32) int32 {
	if mode != nil {
		return *mode
	}
	return fallback
}

func modePtr(fallback int32, mode *int32) *int32 {
	if mode != nil {
		val := *mode
		return &val
	}
	val := fallback
	return &val
}

func formatFileMode(mode int32) string {
	return fmt.Sprintf("%04o", mode)
}

func parseFileMode(raw string) (int32, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("mode is required for chmod")
	}

	base := 10
	if strings.HasPrefix(raw, "0") {
		base = 8
	}

	val, err := strconv.ParseInt(raw, base, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid mode %q", raw)
	}
	if val < 0 || val > 0o7777 {
		return 0, fmt.Errorf("mode out of range: %s", raw)
	}
	return int32(val), nil
}

func applyModeToMountedFile(podSpec *corev1.PodSpec, absolutePath string, mode int32) bool {
	volumes := make(map[string]*corev1.Volume, len(podSpec.Volumes))
	for i := range podSpec.Volumes {
		volumes[podSpec.Volumes[i].Name] = &podSpec.Volumes[i]
	}

	if applyModeToContainerMounts(podSpec.Containers, volumes, absolutePath, mode) {
		return true
	}
	return applyModeToContainerMounts(podSpec.InitContainers, volumes, absolutePath, mode)
}

func applyModeToContainerMounts(containers []corev1.Container, volumes map[string]*corev1.Volume, absolutePath string, mode int32) bool {
	for _, container := range containers {
		for _, volumeMount := range container.VolumeMounts {
			volume, ok := volumes[volumeMount.Name]
			if !ok {
				continue
			}
			if applyModeToVolume(volume, volumeMount, absolutePath, mode) {
				return true
			}
		}
	}
	return false
}

func applyModeToVolume(volume *corev1.Volume, volumeMount corev1.VolumeMount, absolutePath string, mode int32) bool {
	switch {
	case volume.ConfigMap != nil:
		return applyModeToConfigMapVolume(volume.ConfigMap, volumeMount, absolutePath, mode)
	case volume.Secret != nil:
		return applyModeToSecretVolume(volume.Secret, volumeMount, absolutePath, mode)
	case volume.Projected != nil:
		return applyModeToProjectedVolume(volume.Projected, volumeMount, absolutePath, mode)
	default:
		return false
	}
}

func applyModeToConfigMapVolume(source *corev1.ConfigMapVolumeSource, volumeMount corev1.VolumeMount, absolutePath string, mode int32) bool {
	if len(source.Items) > 0 {
		for i := range source.Items {
			if mountTargetPath(volumeMount, source.Items[i].Path) != absolutePath {
				continue
			}
			source.Items[i].Mode = int32Ptr(mode)
			return true
		}
		return false
	}

	if !matchesMountRoot(volumeMount, absolutePath) {
		return false
	}
	source.DefaultMode = int32Ptr(mode)
	return true
}

func applyModeToSecretVolume(source *corev1.SecretVolumeSource, volumeMount corev1.VolumeMount, absolutePath string, mode int32) bool {
	if len(source.Items) > 0 {
		for i := range source.Items {
			if mountTargetPath(volumeMount, source.Items[i].Path) != absolutePath {
				continue
			}
			source.Items[i].Mode = int32Ptr(mode)
			return true
		}
		return false
	}

	if !matchesMountRoot(volumeMount, absolutePath) {
		return false
	}
	source.DefaultMode = int32Ptr(mode)
	return true
}

func applyModeToProjectedVolume(source *corev1.ProjectedVolumeSource, volumeMount corev1.VolumeMount, absolutePath string, mode int32) bool {
	for i := range source.Sources {
		switch {
		case source.Sources[i].ConfigMap != nil:
			if applyModeToConfigMapProjection(source.Sources[i].ConfigMap, volumeMount, absolutePath, mode, source) {
				return true
			}
		case source.Sources[i].Secret != nil:
			if applyModeToSecretProjection(source.Sources[i].Secret, volumeMount, absolutePath, mode, source) {
				return true
			}
		case source.Sources[i].ServiceAccountToken != nil:
			if mountTargetPath(volumeMount, source.Sources[i].ServiceAccountToken.Path) != absolutePath {
				continue
			}
			source.DefaultMode = int32Ptr(mode)
			return true
		}
	}
	return false
}

func applyModeToConfigMapProjection(source *corev1.ConfigMapProjection, volumeMount corev1.VolumeMount, absolutePath string, mode int32, projected *corev1.ProjectedVolumeSource) bool {
	if len(source.Items) > 0 {
		for i := range source.Items {
			if mountTargetPath(volumeMount, source.Items[i].Path) != absolutePath {
				continue
			}
			source.Items[i].Mode = int32Ptr(mode)
			return true
		}
		return false
	}

	if !matchesMountRoot(volumeMount, absolutePath) {
		return false
	}
	projected.DefaultMode = int32Ptr(mode)
	return true
}

func applyModeToSecretProjection(source *corev1.SecretProjection, volumeMount corev1.VolumeMount, absolutePath string, mode int32, projected *corev1.ProjectedVolumeSource) bool {
	if len(source.Items) > 0 {
		for i := range source.Items {
			if mountTargetPath(volumeMount, source.Items[i].Path) != absolutePath {
				continue
			}
			source.Items[i].Mode = int32Ptr(mode)
			return true
		}
		return false
	}

	if !matchesMountRoot(volumeMount, absolutePath) {
		return false
	}
	projected.DefaultMode = int32Ptr(mode)
	return true
}

func int32Ptr(val int32) *int32 {
	return &val
}

func matchesMountRoot(volumeMount corev1.VolumeMount, absolutePath string) bool {
	if volumeMount.SubPath != "" {
		return absolutePath == volumeMount.MountPath
	}
	return matchesMountRootByPath(volumeMount.MountPath, absolutePath)
}

func matchesMountRootByPath(mountPath, absolutePath string) bool {
	if absolutePath == mountPath {
		return true
	}
	prefix := strings.TrimRight(mountPath, "/") + "/"
	return strings.HasPrefix(absolutePath, prefix)
}

func hasMountedFile(files []MountFileEntry, absolutePath string) bool {
	for _, file := range files {
		if file.Path == absolutePath {
			return true
		}
	}
	return false
}

func attachCreatedFileToPodSpec(podSpec *corev1.PodSpec, target createMountTarget, volumeName, configMapName, fileKey, absolutePath string, fileMode int32) error {
	for _, volume := range podSpec.Volumes {
		if volume.Name == volumeName {
			return fmt.Errorf("volume %s already exists", volumeName)
		}
	}

	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: configMapName},
				Items: []corev1.KeyToPath{{
					Key:  fileKey,
					Path: fileKey,
					Mode: int32Ptr(fileMode),
				}},
				DefaultMode: int32Ptr(fileMode),
			},
		},
	})

	switch target.ContainerType {
	case "initContainer":
		return appendMountToContainer(&podSpec.InitContainers, target.ContainerName, volumeName, fileKey, absolutePath)
	default:
		return appendMountToContainer(&podSpec.Containers, target.ContainerName, volumeName, fileKey, absolutePath)
	}
}

func appendMountToContainer(containers *[]corev1.Container, containerName, volumeName, fileKey, absolutePath string) error {
	for i := range *containers {
		if (*containers)[i].Name != containerName {
			continue
		}
		for _, mount := range (*containers)[i].VolumeMounts {
			if mount.MountPath == absolutePath {
				return fmt.Errorf("path %s already mounted in container %s", absolutePath, containerName)
			}
			if mount.Name == volumeName {
				return fmt.Errorf("volume %s already mounted in container %s", volumeName, containerName)
			}
		}
		(*containers)[i].VolumeMounts = append((*containers)[i].VolumeMounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: absolutePath,
			SubPath:   fileKey,
		})
		return nil
	}
	return fmt.Errorf("container %s not found", containerName)
}

func configMapEntryCount(cm *corev1.ConfigMap) int {
	return len(cm.Data) + len(cm.BinaryData)
}

func removeConfigMapVolumesFromPodSpec(podSpec *corev1.PodSpec, configMapName string) {
	volumeNames := make(map[string]struct{})
	filteredVolumes := make([]corev1.Volume, 0, len(podSpec.Volumes))
	for _, volume := range podSpec.Volumes {
		if volume.ConfigMap != nil && volume.ConfigMap.Name == configMapName {
			volumeNames[volume.Name] = struct{}{}
			continue
		}
		filteredVolumes = append(filteredVolumes, volume)
	}
	podSpec.Volumes = filteredVolumes

	if len(volumeNames) == 0 {
		return
	}

	removeVolumeMounts := func(containers []corev1.Container) {
		for i := range containers {
			filteredMounts := make([]corev1.VolumeMount, 0, len(containers[i].VolumeMounts))
			for _, mount := range containers[i].VolumeMounts {
				if _, ok := volumeNames[mount.Name]; ok {
					continue
				}
				filteredMounts = append(filteredMounts, mount)
			}
			containers[i].VolumeMounts = filteredMounts
		}
	}

	removeVolumeMounts(podSpec.Containers)
	removeVolumeMounts(podSpec.InitContainers)
}

func removeMountedFileReferencesFromPodSpec(podSpec *corev1.PodSpec, target *MountFileEntry) {
	volumeNames := findSourceVolumeNames(podSpec.Volumes, target)
	if len(volumeNames) == 0 {
		return
	}

	emptyVolumes := make(map[string]struct{})
	for i := range podSpec.Volumes {
		if _, ok := volumeNames[podSpec.Volumes[i].Name]; !ok {
			continue
		}
		if removeKeyFromVolume(&podSpec.Volumes[i], target) {
			emptyVolumes[podSpec.Volumes[i].Name] = struct{}{}
		}
	}

	removeMountsByAbsolutePath := func(containers []corev1.Container) {
		for i := range containers {
			filteredMounts := make([]corev1.VolumeMount, 0, len(containers[i].VolumeMounts))
			for _, mount := range containers[i].VolumeMounts {
				if _, ok := volumeNames[mount.Name]; ok && mountTargetPath(mount, target.RelativePath) == target.Path {
					continue
				}
				filteredMounts = append(filteredMounts, mount)
			}
			containers[i].VolumeMounts = filteredMounts
		}
	}

	removeMountsByAbsolutePath(podSpec.Containers)
	removeMountsByAbsolutePath(podSpec.InitContainers)

	usedVolumes := mountedVolumeNames(podSpec)
	filteredVolumes := make([]corev1.Volume, 0, len(podSpec.Volumes))
	for _, volume := range podSpec.Volumes {
		if _, empty := emptyVolumes[volume.Name]; empty {
			if _, mounted := usedVolumes[volume.Name]; !mounted {
				continue
			}
		}
		filteredVolumes = append(filteredVolumes, volume)
	}
	podSpec.Volumes = filteredVolumes
}

func findSourceVolumeNames(volumes []corev1.Volume, target *MountFileEntry) map[string]struct{} {
	result := make(map[string]struct{})
	for _, volume := range volumes {
		switch target.SourceType {
		case "configMap":
			if volume.ConfigMap != nil && volume.ConfigMap.Name == target.SourceName {
				result[volume.Name] = struct{}{}
			}
		case "secret":
			if volume.Secret != nil && volume.Secret.SecretName == target.SourceName {
				result[volume.Name] = struct{}{}
			}
		}
	}
	return result
}

func removeKeyFromVolume(volume *corev1.Volume, target *MountFileEntry) bool {
	switch target.SourceType {
	case "configMap":
		if volume.ConfigMap == nil || len(volume.ConfigMap.Items) == 0 {
			return false
		}
		filteredItems := make([]corev1.KeyToPath, 0, len(volume.ConfigMap.Items))
		for _, item := range volume.ConfigMap.Items {
			if item.Key == target.Key {
				continue
			}
			filteredItems = append(filteredItems, item)
		}
		volume.ConfigMap.Items = filteredItems
		return len(volume.ConfigMap.Items) == 0
	case "secret":
		if volume.Secret == nil || len(volume.Secret.Items) == 0 {
			return false
		}
		filteredItems := make([]corev1.KeyToPath, 0, len(volume.Secret.Items))
		for _, item := range volume.Secret.Items {
			if item.Key == target.Key {
				continue
			}
			filteredItems = append(filteredItems, item)
		}
		volume.Secret.Items = filteredItems
		return len(volume.Secret.Items) == 0
	default:
		return false
	}
}

func mountedVolumeNames(podSpec *corev1.PodSpec) map[string]struct{} {
	result := make(map[string]struct{})
	for _, container := range podSpec.Containers {
		for _, mount := range container.VolumeMounts {
			result[mount.Name] = struct{}{}
		}
	}
	for _, container := range podSpec.InitContainers {
		for _, mount := range container.VolumeMounts {
			result[mount.Name] = struct{}{}
		}
	}
	return result
}

func buildMountFileConfigMapName(workloadName, fileName string) string {
	return buildK8sGeneratedName(workloadName+"-mountfile-"+fileName, 63)
}

func buildMountFileVolumeName(fileName string) string {
	return buildK8sGeneratedName("mountfile-"+fileName, 63)
}

func buildK8sGeneratedName(prefix string, maxLen int) string {
	suffix := strings.ToLower(helper.RandomString(6))
	base := sanitizeK8sName(prefix)
	if base == "" {
		base = "mountfile"
	}
	limit := maxLen - len(suffix) - 1
	if limit < 1 {
		limit = 1
	}
	if len(base) > limit {
		base = strings.Trim(base[:limit], "-")
	}
	if base == "" {
		base = "m"
	}
	return base + "-" + suffix
}

func sanitizeK8sName(raw string) string {
	raw = strings.ToLower(raw)
	var builder strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			lastDash = false
		default:
			if lastDash {
				continue
			}
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}
