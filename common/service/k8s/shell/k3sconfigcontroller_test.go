package shell

import "testing"

func TestK3sFirstNodeWithTokenKeepsTlsSan(t *testing.T) {
	first := NewK3sConfig(`["server","--token","secret","--tls-san","10.43.196.114","--cluster-init"]`, `{"K3S_TOKEN":"secret"}`, nil)
	if first.IsNotFirstNode() || len(first.GetDefaultTlsSanIp()) != 1 || first.GetDefaultTlsSanIp()[0] != "10.43.196.114" {
		t.Fatalf("first server with token skipped or lost tls-san: %#v", first.GetDefaultTlsSanIp())
	}
	for _, args := range []string{`["server","--server","https://peer:6443","--token","secret"]`, `["server","--server=https://peer:6443","--token","secret"]`} {
		if !NewK3sConfig(args, `{}`, nil).IsNotFirstNode() {
			t.Fatalf("joining server not detected: %s", args)
		}
	}
}

func TestK3sModeReadsConfigYaml(t *testing.T) {
	for _, config := range []string{"cluster-init: true\n", "cluster-init: 'true'\n"} {
		k3s := NewK3sConfig(`["server"]`, `{}`, []byte(config))
		if !k3s.IsClusterInit() || k3s.GetMode() != "2" {
			t.Fatalf("cluster-init in %q was not recognized", config)
		}
	}
	k3s := NewK3sConfig(`["server"]`, `{}`, []byte("cluster-init: false\n"))
	if k3s.IsClusterInit() || k3s.GetMode() != "1" {
		t.Fatal("SQLite config was recognized as embedded etcd")
	}
	if NewK3sConfig(`["server"]`, `{}`, []byte("cluster-init: [\n")).configErr == nil {
		t.Fatal("invalid config.yaml must stop datastore changes")
	}
}

func TestNodeRefreshPreservesDatastoreChange(t *testing.T) {
	observed := map[string]string{"k3s.cluster-init": "false", "k3s.mode": "1"}
	preserveK3sConfigIntent(observed, map[string]string{"k3s.cluster-init": "true"})
	if observed["k3s.cluster-init"] != "true" || observed["k3s.mode"] != "1" {
		t.Fatalf("node refresh changed pending datastore intent: %#v", observed)
	}
	observed = map[string]string{"k3s.cluster-init": "true", "k3s.mode": "2"}
	preserveK3sConfigIntent(observed, map[string]string{"k3s.cluster-init": "false"})
	if observed["k3s.cluster-init"] != "true" {
		t.Fatal("node refresh hid cluster-init enabled in config.yaml")
	}
}

func TestSetK3sTlsSan(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value string
		want  []string
	}{
		{name: "empty removes config", value: "", want: nil},
		{name: "filters empty entries", value: "10.0.0.1, example.com,", want: []string{"10.0.0.1", "example.com"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]interface{}{"tls-san": []string{"old.example.com"}}
			setK3sTlsSan(config, tt.value)
			got, ok := config["tls-san"].([]string)
			if tt.want == nil {
				if ok {
					t.Fatalf("tls-san = %v, want removed", got)
				}
				return
			}
			if !ok || len(got) != len(tt.want) {
				t.Fatalf("tls-san = %v, want %v", config["tls-san"], tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("tls-san = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestMergeK3sTlsSan(t *testing.T) {
	data := map[string]string{
		"k3s.default-tls-san": "10.0.0.1",
		"k3s.tls-san":         "custom.example.com,10.0.0.1",
		"k3s.public-tls-san":  "203.0.113.2,custom.example.com",
	}
	got := mergeK3sTlsSan(data)
	if got != "10.0.0.1,custom.example.com,203.0.113.2" {
		t.Fatalf("merged tls-san = %q", got)
	}
}

func TestDefaultK3sTlsSanIncludesChildService(t *testing.T) {
	t.Setenv("IS_CHILD", "true")
	t.Setenv("K3K_NAMESPACE", "k3k-demo")
	if got := defaultK3sTlsSan([]string{"10.0.0.1", "k3k-demo-service.k3k-demo"}); got != "10.0.0.1,k3k-demo-service.k3k-demo" {
		t.Fatalf("child default tls-san = %q", got)
	}
	t.Setenv("K3K_NAMESPACE", "")
	if got := defaultK3sTlsSan([]string{"10.0.0.1"}); got != "10.0.0.1" {
		t.Fatalf("missing namespace default tls-san = %q", got)
	}
	t.Setenv("IS_CHILD", "false")
	t.Setenv("K3K_NAMESPACE", "k3k-demo")
	if got := defaultK3sTlsSan([]string{"10.0.0.1"}); got != "10.0.0.1" {
		t.Fatalf("root default tls-san = %q", got)
	}
}
