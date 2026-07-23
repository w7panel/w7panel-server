package v1alpha1

func GetK3kAgentName(name string) string {
	return "w7panel-k3k-agent-" + name
}

func GetK3kServer0Name(name string) string {
	return "k3k-" + name + "-server-0"
}

func GetK3kServer0ContainerName(name string) string {
	return "k3k-" + name + "-server"
}
func GetVirtualIngressServiceName(ns, name string) string {
	return ns + "-" + name + "-service-w7"
}
