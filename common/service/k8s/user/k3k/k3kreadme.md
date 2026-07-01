
# 前提
./k8s/pkg/apis/cvm/v1alpha1 目录下是cvm 类型定义文件
common/service/k8s/user/k3k 原有功能目录
app/k3k http api目录

# 原有功能
1. 一个serviceaccount 对应一个github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1 k3k cluster

# 新需求
1. 一个cvm 对应一个github.com/rancher/k3k/pkg/apis/k3k.io/v1alpha1 k3k cluster
2. 一个serviceAccount 对应多个 cvm
3. 一个service account 对应归属一个namespace
4. 一个namespace 下可以有多个cvm

# 补充
.go 文件带cvm 的都是 新需求已实现的

