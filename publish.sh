helm upgrade w7panel-offline ./kodata/charts/k8s-offline \
	 --set mock.upgrade=true --set servicelb.loadBalancerClass=io.cilium/node --set webhook.enabled=true \
	  --set servicelb.port=9090 --set image.tag=v1.1.40.1 --set image.repository=docker.cnb.cool/i0358/zpk --set controller.appWatch=true --set k3k.watch=true  --set mock.upgrade=true \
	--set storage.enabled=false --install


# helm --kubeconfig=/home/workspace/.kube/124.config upgrade w7panel-offline ./kodata/charts/k8s-offline \
# 	 --set mock.upgrade=true --set servicelb.loadBalancerClass=io.cilium/node --set webhook.enabled=true \
# 	  --set servicelb.port=9090 --set image.tag=v1.1.39.25 --set image.repository=docker.cnb.cool/i0358/zpk --set controller.appWatch=true --set k3k.watch=true  --set mock.upgrade=true \
# 	--set storage.enabled=false --install