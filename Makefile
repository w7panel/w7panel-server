
IV ?= v1.1.42.x
tidy:
   go env -w GOPRIVATE=github.com/w7panel/w7panel-ckm
   go mod tidy
build-docker:
   export KO_DOCKER_REPO=docker.cnb.cool/i0358/zpk
   export KO_DEFAULTBASEIMAGE=ccr.ccs.tencentyun.com/afan-public/ubuntu:24.04-offlineui 
   ko build --bare --tags=v1.1.60.26 --tag-only --sbom=none --platform=all   
build-charts:
	helm package ./kodata/charts/k8s-offline

build-metrics:
	helm package ./kodata/charts/k8s-offline-metrics

build-longhorn:
	helm package ./kodata/charts/w7panel-longhorn

test162:
	cp /home/workspace/.kube/162.config /home/workspace/.kube/config

test218:
	cp /home/workspace/.kube/218.config /home/workspace/.kube/config

build-image:
# 	export KO_DOCKER_REPO=docker.cnb.cool/i0358/zpk  
# 	export KO_DEFAULTBASEIMAGE=ccr.ccs.tencentyun.com/afan-public/ubuntu:24.04-offlineui 
# 	ko build --bare --tags=${IV} --tag-only --sbom=none --platform=all 

publish:
	helm upgrade w7panel-offline ./kodata/charts/k8s-offline \
	 --set mock.upgrade=true --set servicelb.loadBalancerClass=io.cilium/node --set webhook.enabled=true \
	  --set servicelb.port=9090 --set image.tag=$(IV) --set image.repository=docker.cnb.cool/i0358/zpk --set controller.appWatch=true --set k3k.watch=true  --set mock.upgrade=true \
	   --set storage.enabled=false --install
