package controller

import "testing"

func TestNewWebDAVIDMapperSkipsChildAgent(t *testing.T) {
	t.Setenv("IS_CHILD", "true")
	if mapper := newWebDAVIDMapper("1", ""); mapper != nil {
		t.Fatal("child agent should not use uid/gid namespace mapper")
	}
}
