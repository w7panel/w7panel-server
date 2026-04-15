helm upgrade w7panel-offline ./kodata/charts/k8s-offline \
	 --set mock.upgrade=true --set servicelb.loadBalancerClass=io.cilium/node --set webhook.enabled=true \
<<<<<<< HEAD
	  --set servicelb.port=9090 --set image.tag=v1.1.44.1 --set image.repository=docker.cnb.cool/i0358/zpk --set controller.appWatch=true --set k3k.watch=true  --set mock.upgrade=true \
=======
	  --set servicelb.port=9090 --set image.tag=v1.1.44.8 --set image.repository=docker.cnb.cool/i0358/zpk --set controller.appWatch=true --set k3k.watch=true  --set mock.upgrade=true \
>>>>>>> dev-v1
	--set storage.enabled=false --install

# https://oneuptime.com/blog/post/2026-01-30-longhorn-volume-snapshots/view
# helm --kubeconfig=/home/workspace/.kube/162.config upgrade w7panel-offline ./kodata/charts/k8s-offline \
# 	 --set mock.upgrade=true --set servicelb.loadBalancerClass=io.cilium/node --set webhook.enabled=true \
# 	  --set servicelb.port=9090 --set image.tag=v1.1.44.6 --set image.repository=docker.cnb.cool/i0358/zpk --set controller.appWatch=true --set k3k.watch=true  --set mock.upgrade=true \
# 	--set storage.enabled=false --install