package agentpod

import (
	"fmt"
	"net"
	"strings"
	"sync"
)

var byNode sync.Map

func Register(nodeIP, podIP string) error {
	node := net.ParseIP(strings.TrimSpace(nodeIP))
	pod := net.ParseIP(strings.TrimSpace(podIP))
	if node == nil || pod == nil {
		return fmt.Errorf("invalid nodeIp or podIp")
	}
	byNode.Store(node.String(), pod.String())
	return nil
}

func Lookup(nodeIP string) (string, bool) {
	node := net.ParseIP(strings.TrimSpace(nodeIP))
	if node == nil {
		return "", false
	}
	podIP, ok := byNode.Load(node.String())
	if !ok {
		return "", false
	}
	return podIP.(string), true
}
