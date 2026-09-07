package console

import (
	"testing"

	"github.com/w7panel/w7panel/common/service/config"
	"github.com/w7panel/w7panel/common/service/k8s"
	// "github.com/w7panel/w7panel/common/service/config"
)

func TestRefreshUseCdToken(t *testing.T) {

	repository := config.NewW7ConfigRepository(k8s.NewK8sClientInner())

	w7config, err := repository.Get("admin")
	if err != nil {
		t.Error(err)
	}
	token := w7config.ThirdpartyCDToken

	cdclient := NewConsoleCdClient(token)

	_, err = cdclient.RefreshToken()
	if err != nil {
		t.Error(err)
	}
}

func TestAccessTokenToCdToken(t *testing.T) {

	SetConsoleApi("http://172.16.1.18:9004")
	atoken, err := OpenIdToCloudAccessToken("N0mGBDIKIWtflauISqnHeQ")
	if err != nil {
		t.Error(err)
		return
	}
	cdToken, err := AccessTokenToCDToken(atoken.Token)
	if err != nil {
		t.Error(err)
	}
	t.Log(cdToken)
}
