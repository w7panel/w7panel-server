package bootstrap

import (
	"github.com/w7panel/w7panel/common/service/k8s"
	bootstrapv1 "github.com/w7panel/w7panel/k8s/pkg/apis/bootstrap/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupControllers registers the two bootstrap controllers with the shared manager.
func SetupControllers(mgr ctrl.Manager, sdk *k8s.Sdk) error {
	if err := bootstrapv1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	if err := setupProfileController(mgr); err != nil {
		return err
	}
	return setupArtifactController(mgr, sdk)
}
