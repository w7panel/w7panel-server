package bootstrap

import (
	"github.com/w7panel/w7panel/common/service/k8s"
	installationv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrapinstallation/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupControllers registers the BootstrapInstallation controller with the shared manager.
func SetupControllers(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	if err := installationv1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	return setupInstallationController(mgr, sdk)
}
