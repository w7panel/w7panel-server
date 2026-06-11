package webhook

import (
	"log/slog"

	"github.com/w7panel/w7panel/common/service/k8s"
	"github.com/w7panel/w7panel/common/service/k8s/appgroup"
	microapp "github.com/w7panel/w7panel/k8s/pkg/apis/microapp/v1alpha1"
	sigclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func SetControllerReference(microApp *microapp.MicroApp, client sigclient.Client) bool {
	if !controllerutil.HasControllerReference(microApp) {
		// return admission.Allowed("")
		group, err := appgroup.GetAppgroup(microApp.Name, microApp.Namespace, client)
		if err != nil {
			return false
		}
		err = controllerutil.SetControllerReference(group, microApp, k8s.GetScheme())
		if err != nil {
			slog.Error("SetControllerReference error", "error", err)
			return false
		}
		return true
	}
	return false
}
