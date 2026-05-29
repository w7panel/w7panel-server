#!/bin/bash


#git clone https://gh-proxy.org/https://github.com/kubernetes/code-generator.git
#https://juejin.cn/post/7373986431851642917
# go get sigs.k8s.io/controller-tools/cmd/controller-gen
# go install sigs.k8s.io/controller-tools/cmd/controller-gen
#cd ../ && controller-gen crd paths=./... output:crd:dir=kodata/crds

<<<<<<< HEAD
/home/workspace/go/bin/controller-gen crd paths=./k8s/pkg/apis/cvm/v1alpha1 output:crd:dir=kodata/crds
/home/workspace/go/bin/controller-gen crd:allowDangerousTypes=true paths=./k8s/pkg/apis/costconfig/v1alpha1 output:crd:dir=kodata/crds
/home/workspace/go/bin/controller-gen crd:allowDangerousTypes=true paths=./k8s/pkg/apis/costconfig/v1alpha1 output:crd:dir=config/crd/bases
=======
/home/workspace/go/bin/controller-gen crd paths=./k8s/pkg/apis/... output:crd:dir=kodata/crds
>>>>>>> dev-v1
