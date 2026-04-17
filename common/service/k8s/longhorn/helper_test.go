package longhorn

import "testing"

func Test_IsExpanding(t *testing.T) {

	volume, err := lclient.GetVolume("pvc-7c650653-00f8-49c8-8063-e1608bd3fdaa")
	if err != nil {
		t.Error(err)
	}
	engineList, err := lclient.GetEngineList()
	if err != nil {
		t.Error(err)
	}
	isExpanding, _ := IsVolumeExpanding(volume, engineList)
	t.Log(isExpanding)
}
