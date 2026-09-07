// nolint
package console

import (
	"os"
	"testing"
)

func TestRefreshToken(t *testing.T) {
	// os.Setenv("HELM_VERSION", "1.0.63.1")
	// SetConsoleApi("http://172.16.1.116:9004")
	RefreshCDToken()
}

func TestRefreshCDToken(t *testing.T) {
	// Setup
	os.Setenv("HELM_VERSION", "1.0.63.1")
	// SetConsoleApi("http://172.16.1.116:9004")
	RefreshCDToken()

}
