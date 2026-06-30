package console

import (
	"os"
	"testing"

	"github.com/w7panel/w7panel/common/service/k8s"
)

func TestPath(t *testing.T) {
	os.Setenv("DEBUG", "true")
	err := PatchAppId(k8s.NewK8sClient().Sdk, &AppSecret{AppId: "1", AppSecret: "2"}, "cs-zyy-rxnnhxjl", "default", "cs-zyy")
	if err != nil {
		t.Errorf("PatchAppId() error = %v", err)
	}
}

func TestRegisterSite(t *testing.T) {
	// os.Setenv("USER_AGFNT", "we7test-beta")
	os.Setenv("DEBUG", "true")
	err := RegisterLicenseSite("admin")
	if err != nil {
		t.Errorf("RegisterSite() error = %v", err)
	}
}

func TestRegisterUserOpenId(t *testing.T) {
	// os.Setenv("USER_AGFNT", "we7test-beta")
	os.Setenv("DEBUG", "true")
	result, err := RegisterSiteZpkOpenId("host1.fan.sz.w7.com", "test-a", "uKnkpN39QyZZ0CTz9JULiQ")
	if err != nil {
		t.Errorf("RegisterSite() error = %v", err)
	}
	t.Log(result)
}

// 修改原函数以接受接口作为参数进行测试
