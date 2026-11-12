package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	helmservice "github.com/w7panel/w7panel/common/service/helm"
	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	"github.com/w7panel/w7panel/common/service/k8s/zpk/logic"
	appv1 "github.com/w7panel/w7panel/k8s/pkg/apis/appgroup/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (self Zpk) UpgradeCRDs(http *gin.Context) {
	params := logic.InstallRequest{}
	if !self.Validate(http, &params) {
		return
	}
	token := http.MustGet("k8s_token").(string)
	if !k8s.NewK8sToken(token).IsFounder() {
		self.JsonResponseWithServerError(http, errors.New("only founder can upgrade"))
		return
	}
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	result, err := logic.CheckUpgradeCRDs(http.Request.Context(), sdk, token, params)
	if err != nil {
		self.JsonResponseWithServerError(http, err)
		return
	}
	self.JsonResponseWithoutError(http, result)
}

type uninstallCRDRequest struct {
	Namespace        string   `json:"namespace" binding:"required"`
	ReleaseName      string   `json:"releaseName" binding:"required"`
	ReleaseNames     []string `json:"releaseNames"`
	PreservePrepared bool     `json:"preservePrepared"`
	ForceSharedCRDs  bool     `json:"forceSharedCrds"`
}

func preparedCleanupNames(group *appv1.AppGroup) ([]string, error) {
	raw := group.Annotations[helmservice.CRDCleanupAnnotation]
	if raw == "" {
		return nil, nil
	}
	if !helmservice.VerifyCRDCleanupSignature(group.Namespace, string(group.UID), raw+"\n"+group.Annotations[helmservice.CRDCleanupAfterGroupsAnnotation]+"\n"+group.Annotations[helmservice.CRDCleanupForceSharedAnnotation], group.Annotations[helmservice.CRDCleanupSignatureAnnotation]) {
		return nil, errors.New("invalid prepared CRD cleanup")
	}
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		return nil, err
	}
	return names, nil
}

func validateUninstallDependents(groups map[string]*appv1.AppGroup, namespace, root string) error {
	reachable := map[string]bool{root: true}
	for range groups {
		for name, group := range groups {
			if reachable[name] {
				continue
			}
			for _, dependency := range group.Spec.Dependencies {
				if (dependency.Namespace == "" || dependency.Namespace == namespace) && reachable[dependency.Name] {
					reachable[name] = true
					break
				}
			}
		}
	}
	if len(reachable) != len(groups) {
		return errors.New("uninstall list contains an application unrelated to the root")
	}
	return nil
}

func omittedUninstallDependent(groups []appv1.AppGroup, selected map[string]bool, namespace string) string {
	for _, group := range groups {
		if selected[group.Name] {
			continue
		}
		for _, dependency := range group.Spec.Dependencies {
			if (dependency.Namespace == "" || dependency.Namespace == namespace) && selected[dependency.Name] {
				return group.Name
			}
		}
	}
	return ""
}

func (self Zpk) UninstallCRDCheck(httpContext *gin.Context) {
	params := uninstallCRDRequest{}
	if !self.Validate(httpContext, &params) {
		return
	}
	token := httpContext.MustGet("k8s_token").(string)
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	result, err := helmservice.NewHelm(sdk).UninstallCRDs(httpContext.Request.Context(), params.Namespace, params.ReleaseName)
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	if group, err := appgroup.GetAppgroupUseSdk(params.ReleaseName, params.Namespace, sdk); err == nil {
		prepared, err := preparedCleanupNames(group)
		if err != nil {
			self.JsonResponseWithServerError(httpContext, err)
			return
		}
		for _, name := range prepared {
			if !slices.Contains(result.Names, name) {
				result.Names = append(result.Names, name)
				result.Risks = append(result.Risks, name+" is pending from an earlier uninstall attempt")
			}
		}
		sort.Strings(result.Names)
	} else if !apierrors.IsNotFound(err) {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	self.JsonResponseWithoutError(httpContext, gin.H{
		"names": result.Names, "risks": result.Risks,
		"canDelete": k8s.NewK8sToken(token).IsFounder(),
	})
}

func (self Zpk) PrepareUninstallCRDs(httpContext *gin.Context) {
	params := uninstallCRDRequest{}
	if !self.Validate(httpContext, &params) {
		return
	}
	token := httpContext.MustGet("k8s_token").(string)
	if !k8s.NewK8sToken(token).IsFounder() {
		self.JsonResponseWithError(httpContext, errors.New("only founder can delete CRDs"), http.StatusForbidden)
		return
	}
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	group, err := appgroup.GetAppgroupUseSdk(params.ReleaseName, params.Namespace, sdk)
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	if len(params.ReleaseNames) == 0 || len(params.ReleaseNames) > 100 {
		self.JsonResponseWithError(httpContext, errors.New("invalid uninstall application list"), http.StatusBadRequest)
		return
	}
	includesRoot := false
	seen := map[string]bool{}
	groups := map[string]*appv1.AppGroup{}
	for _, name := range params.ReleaseNames {
		if name == "" || seen[name] {
			self.JsonResponseWithError(httpContext, errors.New("invalid uninstall application list"), http.StatusBadRequest)
			return
		}
		seen[name] = true
		includesRoot = includesRoot || name == group.Name
		dependentGroup, err := appgroup.GetAppgroupUseSdk(name, params.Namespace, sdk)
		if err != nil {
			self.JsonResponseWithServerError(httpContext, err)
			return
		}
		groups[name] = dependentGroup
	}
	if !includesRoot {
		self.JsonResponseWithError(httpContext, errors.New("uninstall list must include the root application"), http.StatusBadRequest)
		return
	}
	if err := validateUninstallDependents(groups, params.Namespace, group.Name); err != nil {
		self.JsonResponseWithError(httpContext, err, http.StatusBadRequest)
		return
	}
	client, err := sdk.ToSigClient()
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	allGroups := &appv1.AppGroupList{}
	if err := client.List(httpContext.Request.Context(), allGroups, sigclient.InNamespace(params.Namespace)); err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	if omitted := omittedUninstallDependent(allGroups.Items, seen, params.Namespace); omitted != "" {
		self.JsonResponseWithError(httpContext, errors.New("uninstall list omits dependent application "+omitted), http.StatusConflict)
		return
	}
	names := map[string]bool{}
	prepared, err := preparedCleanupNames(group)
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	for _, name := range prepared {
		names[name] = true
	}
	helm := helmservice.NewHelm(sdk)
	for _, releaseName := range params.ReleaseNames {
		check, err := helm.UninstallCRDs(httpContext.Request.Context(), params.Namespace, releaseName)
		if err != nil {
			self.JsonResponseWithServerError(httpContext, err)
			return
		}
		if len(check.Risks) > 0 && !params.ForceSharedCRDs {
			self.JsonResponseWithError(httpContext, errors.New("shared CRDs require explicit confirmation"), http.StatusConflict)
			return
		}
		for _, name := range check.Names {
			names[name] = true
		}
	}
	list := make([]string, 0, len(names))
	for name := range names {
		list = append(list, name)
	}
	sort.Strings(list)
	encoded, _ := json.Marshal(list)
	dependents := make([]string, 0, len(params.ReleaseNames))
	dependentNames := map[string]bool{}
	for _, name := range params.ReleaseNames {
		if name != group.Name {
			dependentNames[name] = true
		}
	}
	if group.Annotations[helmservice.CRDCleanupAfterGroupsAnnotation] != "" {
		var previous []string
		if err := json.Unmarshal([]byte(group.Annotations[helmservice.CRDCleanupAfterGroupsAnnotation]), &previous); err != nil {
			self.JsonResponseWithServerError(httpContext, err)
			return
		}
		for _, name := range previous {
			if name != group.Name {
				dependentNames[name] = true
			}
		}
	}
	for name := range dependentNames {
		dependents = append(dependents, name)
	}
	sort.Strings(dependents)
	after, _ := json.Marshal(dependents)
	forceShared := strconv.FormatBool(params.ForceSharedCRDs)
	signature, err := helmservice.CRDCleanupSignature(group.Namespace, string(group.UID), string(encoded)+"\n"+string(after)+"\n"+forceShared)
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	if group.Annotations == nil {
		group.Annotations = map[string]string{}
	}
	group.Annotations[helmservice.CRDCleanupAnnotation] = string(encoded)
	group.Annotations[helmservice.CRDCleanupAfterGroupsAnnotation] = string(after)
	group.Annotations[helmservice.CRDCleanupForceSharedAnnotation] = forceShared
	group.Annotations[helmservice.CRDCleanupSignatureAnnotation] = signature
	if err := client.Update(httpContext.Request.Context(), group); err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	self.JsonResponseWithoutError(httpContext, gin.H{"names": list})
}

func (self Zpk) UninstallWithCRDs(httpContext *gin.Context) {
	params := uninstallCRDRequest{}
	if !self.Validate(httpContext, &params) {
		return
	}
	token := httpContext.MustGet("k8s_token").(string)
	if params.PreservePrepared && !k8s.NewK8sToken(token).IsFounder() {
		self.JsonResponseWithError(httpContext, errors.New("only founder can delete CRDs"), http.StatusForbidden)
		return
	}
	sdk, err := k8s.NewK8sClient().Channel(token)
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	group, err := appgroup.GetAppgroupUseSdk(params.ReleaseName, params.Namespace, sdk)
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	client, err := sdk.ToSigClient()
	if err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	annotationChanged := false
	if params.PreservePrepared && !helmservice.VerifyCRDCleanupSignature(group.Namespace, string(group.UID), group.Annotations[helmservice.CRDCleanupAnnotation]+"\n"+group.Annotations[helmservice.CRDCleanupAfterGroupsAnnotation]+"\n"+group.Annotations[helmservice.CRDCleanupForceSharedAnnotation], group.Annotations[helmservice.CRDCleanupSignatureAnnotation]) {
		self.JsonResponseWithError(httpContext, errors.New("CRD cleanup was not prepared"), http.StatusConflict)
		return
	}
	if group.Annotations != nil && !params.PreservePrepared && group.Annotations[helmservice.CRDCleanupAnnotation] != "" {
		delete(group.Annotations, helmservice.CRDCleanupAnnotation)
		delete(group.Annotations, helmservice.CRDCleanupAfterGroupsAnnotation)
		delete(group.Annotations, helmservice.CRDCleanupForceSharedAnnotation)
		delete(group.Annotations, helmservice.CRDCleanupSignatureAnnotation)
		annotationChanged = true
	}
	if annotationChanged {
		if err := client.Update(httpContext.Request.Context(), group); err != nil {
			self.JsonResponseWithServerError(httpContext, err)
			return
		}
	}
	if err := client.Delete(httpContext.Request.Context(), group, &sigclient.DeleteOptions{}); err != nil {
		self.JsonResponseWithServerError(httpContext, err)
		return
	}
	self.JsonResponseWithoutError(httpContext, gin.H{"accepted": true})
}
