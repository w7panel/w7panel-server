package longhorn

import (
	"testing"
)

func TestWebHookStorageClass(t *testing.T) {
	nodes, err := GetLonghornNodeList()
	if err != nil {
		t.Fatal(err)
	}
	deleteNotSelectorStorageClass(nodes)
}

func TestDeleleNode(t *testing.T) {
	err := lclient.DeleteNode("server1")
	if err != nil {
		t.Fatal(err)
	}
}
