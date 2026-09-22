package controller

import "testing"

func TestDecodeCopilotObject(t *testing.T) {
	object, err := decodeCopilotObject("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: example\n  namespace: default\ndata:\n  key: value\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := resourceRef(object); got != "ConfigMap/default/example" {
		t.Fatalf("resourceRef() = %q", got)
	}
}

func TestDecodeCopilotObjectRejectsMultipleResources(t *testing.T) {
	_, err := decodeCopilotObject("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: first\n---\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: second\n")
	if err == nil {
		t.Fatal("expected multiple manifests to be rejected")
	}
}
