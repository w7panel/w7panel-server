

sudo ctr -n k8s.io images prune --all

sudo ctr -n k8s.io images pull 127.0.0.1:9007/afan-public/nginx:latest

docker save -o test.tar ccr.ccs.tencentyun.com/afan-public/nginx:latest

sudo ctr -n k8s.io images import ./test.tar



sudo chmod 666 /run/containerd/containerd.sock

ctr run --rm ccr.ccs.tencentyun.com/afan-public/nginx:test nginx-test1

ctr run --rm registry.local.w7.cc/afan-public/n:v1 nginx-test123

sudo ctr task exec --exec-id shell -t nginx-test1 /bin/bash

helm upgrade w7panel-offline ./kodata/charts/k8s-offline \
	 --set mock.upgrade=true --set servicelb.loadBalancerClass=io.cilium/node --set webhook.enabled=true \
	  --set servicelb.port=9090 --set image.tag=v1.1.49.1 --set image.repository=docker.cnb.cool/i0358/zpk --set controller.appWatch=true --set k3k.watch=true  --set mock.upgrade=true \
	--set storage.enabled=false --install 

# https://oneuptime.com/blog/post/2026-01-30-longhorn-volume-snapshots/view
# helm --kubeconfig=/home/workspace/.kube/162.config upgrade w7panel-offline ./kodata/charts/k8s-offline \
# 	 --set mock.upgrade=true --set servicelb.loadBalancerClass=io.cilium/node --set webhook.enabled=true \
# 	  --set servicelb.port=9090 --set image.tag=v1.1.44.6 --set image.repository=docker.cnb.cool/i0358/zpk --set controller.appWatch=true --set k3k.watch=true  --set mock.upgrade=true \
# 	--set storage.enabled=false --install