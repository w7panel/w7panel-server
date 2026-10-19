package console

import (
	"testing"
)

type mockLicenseClient struct {
	cleanErr error
}

func (m *mockLicenseClient) CleanLicense() error {
	return m.cleanErr
}

func TestCleanLicenseCert(t *testing.T) {
	cleanLicenseCert(true)
}

func TestVerifyDefaultLicense(t *testing.T) {
	VerifyDefaultLicense(false)
}
