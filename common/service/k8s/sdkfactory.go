package k8s

import (
	"context"
	"errors"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type singleton struct {
	*Sdk
	sdks    map[string]*Sdk
	expires map[string]int64 // 缓存过期时间(unix timestamp)
	mu      sync.Mutex
}

// instance 是一个包级别的变量，用于保存唯一的单例对象
var instance *singleton

// once 是一个用于确保初始化操作只执行一次的 sync.Once 对象
var once sync.Once

type Channel interface {
	Channel(token string) (*Sdk, error)
}

// GetInstance 方法返回唯一的单例对象
func NewK8sClient() *singleton {
	// 使用 sync.Once 确保初始化操作只执行一次
	once.Do(func() {
		sdk := NewK8sClientInner()
		instance = &singleton{}
		instance.Sdk = sdk
		instance.sdks = make(map[string]*Sdk)
		instance.expires = make(map[string]int64)
	})
	instance.Sdk = NewK8sClientInner()
	return instance
}

func (s *singleton) GetSdk() *Sdk {
	return s.Sdk
}

func (s *singleton) ChannelLocal(token string, forceLocal bool) (*Sdk, error) {

	if forceLocal {
		result, err := s.loadFromCache(token)
		return result, err
	}
	result, err := s.Channel(token)
	return result, err
}

func (s *singleton) Channel(token string) (*Sdk, error) {
	tokenobj := NewK8sToken(token)
	isK3k := tokenobj.IsK3kCluster()

	if isK3k {
		result, err := s.GetK3kClusterSdk(tokenobj)
		return result, err
	}

	result, err := s.loadFromCache(token)
	return result, err
}
func (s *singleton) loadFromCache(token string) (*Sdk, error) {

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sdks) > 100 {
		s.sdks = make(map[string]*Sdk)
	}
	sdk, ok := s.sdks[token]
	if !ok {
		sdk2, err := s.Sdk.Channel(token)
		if err != nil {
			return nil, err
		}
		s.sdks[token] = sdk2
		sdk = sdk2

	}
	return sdk, nil
}
func (s *singleton) GetK3kClusterSdkByConfig(k3kconfig *K3kConfig) (*Sdk, error) {
<<<<<<< HEAD
	return s.GetK3kClusterSdkByConfig0(k3kconfig, true)
}
func (s *singleton) GetK3kClusterSdkByConfig0(k3kconfig *K3kConfig, createToken bool) (*Sdk, error) {
=======
>>>>>>> dev-v1

	s.mu.Lock()
	defer s.mu.Unlock()

	cacheKey := k3kconfig.Name
	if k3kconfig.CvmName != "" {
		cacheKey = k3kconfig.Name + "-" + k3kconfig.CvmName
	}
	// 检查缓存是否存在且未过期
	// cacheCheckStart := time.Now()
<<<<<<< HEAD
	if sdk, ok := s.sdks[cacheKey]; ok {
		if expireTime, ok := s.expires[cacheKey]; ok && expireTime > time.Now().Unix() {
=======
	if sdk, ok := s.sdks[k3kconfig.Name]; ok {
		if expireTime, ok := s.expires[k3kconfig.Name]; ok && expireTime > time.Now().Unix() {
>>>>>>> dev-v1

			return sdk, nil
		}
	}

	// toSigStart := time.Now()
	sigClient, err := s.Sdk.ToSigClient()

	if err != nil {
		return nil, err
	}

	kubeconfig, err := GetK3kKubeConfig(sigClient, k3kconfig)

	if err != nil {
		return nil, err
	}

	clientconfig := clientcmd.NewDefaultClientConfig(*kubeconfig, &clientcmd.ConfigOverrides{})
	restConfig, err := clientconfig.ClientConfig()

	if err != nil {
		return nil, err
	}

	sdk, err := NewForRestConfig(restConfig, "default")

	if err != nil {
		return nil, err
	}
	//k3kcvmcontroller 需要先创建k3kconfig.Name 的serviceaccount 才能CreateTokenRequest
	if !createToken {
		return sdk, nil
	}

<<<<<<< HEAD
	// token, err := sdk.CreateTokenRequest(k3kconfig.Name, 7200, []string{})
	//k3k cluster 使用addons挂载 不能使用动态k3kconfig.Name 写死default
	// addons 不能使用 k3kcvmcontroller.go 动态绑定rolebings 改回k3kconfig.Name
=======
>>>>>>> dev-v1
	token, err := sdk.CreateTokenRequest(k3kconfig.Name, 7200, []string{})

	if err != nil {
		return nil, err
	}

	// 缓存结果并设置过期时间(1小时)

	result, err := sdk.Channel(token)

	if err == nil {
		s.sdks[cacheKey] = result
		s.expires[cacheKey] = time.Now().Add(time.Hour).Unix()
	}

	return result, err
}
func (s *singleton) GetK3kClusterSdk(k8stoken *K8sToken) (*Sdk, error) {

	k3kconfig, err := k8stoken.GetK3kConfig()
	if err != nil {
		return nil, err
	}

	result, err := s.GetK3kClusterSdkByConfig0(k3kconfig, true)
	return result, err
}

func (s *singleton) Clear(k3kName string, cvmName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sdks, k3kName)
	delete(s.expires, k3kName)
	if cvmName != "" {
		cacheKey := k3kName + "-" + cvmName
		delete(s.sdks, cacheKey)
		delete(s.expires, cacheKey)
	}
}

// GetK3kKubeConfig 从Kubernetes集群中获取k3k的kubeconfig配置
//
// 参数：
//
//	sigClient: Kubernetes的client，用于与Kubernetes集群进行交互
//	k3kconfig: K3kConfig结构体，包含了k3k集群的配置信息
//
// 返回值：
//
//	*clientcmdapi.Config: 解析后的kubeconfig配置
//	error: 如果获取kubeconfig失败，则返回错误信息
func GetK3kKubeConfig(sigClient client.Client, k3kconfig *K3kConfig) (*clientcmdapi.Config, error) {
<<<<<<< HEAD
=======
	// startTime := time.Now()
>>>>>>> dev-v1

	secret := &corev1.Secret{}
	kubeConfigName := "k3k-" + k3kconfig.CvmName + "-kubeconfig"

<<<<<<< HEAD
	err := sigClient.Get(context.TODO(), types.NamespacedName{Name: kubeConfigName, Namespace: k3kconfig.Namespace}, secret)
=======
	// getSecretStart := time.Now()
	err := sigClient.Get(context.TODO(), types.NamespacedName{Name: kubeConfigName, Namespace: k3kconfig.Namespace}, secret)
	// log.Printf("[PERF] GetK3kKubeConfig - Get Secret took %v", time.Since(getSecretStart))
>>>>>>> dev-v1
	if err != nil {
		return nil, err
	}

	kubeconfigYaml := secret.Data["kubeconfig.yaml"]
	if len(kubeconfigYaml) == 0 {
		return nil, errors.New("kubeconfig.yaml is empty")
	}
<<<<<<< HEAD
	kubeconfig, err := clientcmd.Load(kubeconfigYaml)
=======

	// loadStart := time.Now()
	kubeconfig, err := clientcmd.Load(kubeconfigYaml)
	// log.Printf("[PERF] GetK3kKubeConfig - Load kubeconfig took %v", time.Since(loadStart))
	// log.Printf("[PERF] GetK3kKubeConfig total time %v", time.Since(startTime))
>>>>>>> dev-v1
	return kubeconfig, err
}

func NewForCmdConfig(kubeconfig *clientcmdapi.Config) (*Sdk, error) {
	clientconfig := clientcmd.NewDefaultClientConfig(*kubeconfig, &clientcmd.ConfigOverrides{})
	restConfig, err := clientconfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	sdk, err := NewForRestConfig(restConfig, "default")
	if err != nil {
		return nil, err
	}
	return sdk, nil
}
