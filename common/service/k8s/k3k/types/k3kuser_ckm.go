package types

import (
	"encoding/json"

	cvmv1alpha1 "github.com/w7panel/w7panel/common/service/k8s/ckm/api/v1alpha1"
)

/*
*	同步ckm 中的权限数据到k3k

	w7.cc/file-editor: 'true'

	w7.cc/login-time: '2026-05-25 22:20:19'
	w7.cc/menu: >-
	    ["cluster","cluster/panel","app","app/apps","app/apps/add","app/apps/edit","app/apps/delete","app/cronjob","app/cronjob/add","app/cronjob/edit","app/cronjob/delete","app/rvproxy","app/rvproxy/add","app/rvproxy/edit","app/rvproxy/delete","storage","storage/disk","storage/disk/add","storage/disk/edit","storage/disk/delete","storage/zone","system","system/cloud","system/cost","person/order-center","person/cost-center"]
	w7.cc/menu-name: dqobvuhu
	w7.cc/quota-limit: >-
	    {"storageclass":"union1","hard":{"cpu":"2","memory":"4","bandwidth":"1","requests.storage":"10"}}
	w7.cc/quota-limit-name: vljoefug
	w7.cc/web-shell: 'true'
*/
func (u *k3kUser) ReplaceCkm(ckm *cvmv1alpha1.Ckm) {
	if ckm.Annotations == nil {
		return
	}
	// u.Annotations[W7_MENU_NAME] = menu.Name
	u.Annotations[K3K_DEBUG] = ckm.Annotations[K3K_DEBUG]
	u.Annotations[W7_MENU] = ckm.Annotations[W7_MENU]
	u.Annotations[W7_WEB_SHELL] = ckm.Annotations[W7_WEB_SHELL]
	u.Annotations[W7_FILE_EDITTOR] = ckm.Annotations[W7_FILE_EDITTOR]
	if ckm.Spec.Rescue { //维护模式 只给这几个权限 只能删除 不能新建编辑
		whMenu := []string{"cluster", "cluster/panel", "cluster/resource", "app", "app/apps", "app/apps/delete"}
		json, _ := json.Marshal(whMenu)
		u.Annotations[W7_MENU] = string(json)

	}

}
