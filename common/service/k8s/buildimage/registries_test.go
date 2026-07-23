package buildimage

import "testing"

func Test_mirrorMapToStr(t *testing.T) {

	test := mirrorMapToStr("127.0.0.1:8000")
	t.Log(test)
}
