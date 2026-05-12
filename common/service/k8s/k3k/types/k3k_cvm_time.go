package types

import (
	"fmt"
	"log/slog"
	"time"

	v1alpha1 "cnb.cool/i0358/ai-cvm/api/v1alpha1"
)

type k3kCvmTime struct {
	*v1alpha1.Cvm
}

func Newk3kCvmTime(cvm *v1alpha1.Cvm) *k3kCvmTime {
	return &k3kCvmTime{Cvm: cvm}
}

func (u *k3kCvmTime) changeExpireTime(hour int) {
	expireTime, err := u.GetExpireTime()
	if err != nil {
		expireTime = time.Now().Add(time.Hour * time.Duration(hour))
	}
	if err == nil {
		if expireTime.Before(time.Now()) {
			expireTime = time.Now()
		}
		expireTime = expireTime.Add(time.Hour * time.Duration(hour))
	}
	u.Spec.ExpireTime = expireTime.Format("2006-01-02 15:04:05")
	// u.Annotations[K3K_EXPIRE_TIME] = expireTime.Format("2006-01-02 15:04:05")
}

func (u *k3kCvmTime) GetExpireTime() (time.Time, error) {
	expireTimeStr := u.Spec.ExpireTime
	if expireTimeStr == "" {
		return time.Time{}, fmt.Errorf("expire time not set")
	}
	return time.Parse("2006-01-02 15:04:05", expireTimeStr)
}

func (u *k3kCvmTime) IsExpired() bool {
	expireTime, err := u.GetExpireTime()
	if err != nil {
		return false // 如果没有设置过期时间，默认不过期
	}
	return time.Now().After(expireTime)
}

func (u *k3kCvmTime) HasExpireTime() bool {
	return u.Spec.ExpireTime != ""
}

func (u *k3kCvmTime) HasPendingRecycleTime() bool {
	return u.Spec.RecycleTime != ""

}

// 获取待回收时间
func (u *k3kCvmTime) GetPendingRecycleTime() (time.Time, error) {
	recycleTime := u.Spec.RecycleTime
	if recycleTime == "" {
		expireTime, err := u.GetExpireTime()
		if err != nil {
			return time.Time{}, fmt.Errorf("pending recycle time not set")
		}
		return expireTime.Add(3 * 24 * time.Hour), nil
	}
	return time.Parse("2006-01-02 15:04:05", recycleTime)
}
func (u *k3kCvmTime) SetPendingRecycleTime() {
	if u.HasPendingRecycleTime() {
		return
	}
	defaultTime := time.Now().Add(3 * 24 * time.Hour)
	if u.HasExpireTime() {
		expireTime, err := u.GetExpireTime()
		if err == nil {
			defaultTime = expireTime.Add(72 * time.Hour)
		}
	}
	u.Spec.RecycleTime = defaultTime.Format("2006-01-02 15:04:05")
	// u.Annotations[K3K_PENDING_RECYCLE_TIME] = defaultTime.Format("2006-01-02 15:04:05")
}

func (u *k3kCvmTime) DelPendingRecycleTime() {
	u.Spec.RecycleTime = ""
	// delete(u.Annotations, K3K_PENDING_RECYCLE_TIME)
}

// 检查待回收是否超过3天
func (u *k3kCvmTime) IsPendingRecycleExpired() bool {
	// _, ok := u.Annotations[K3K_EXPIRE_TIME]
	// if !ok {
	// 	return false //不存在就不计算过期
	// }
	pendingTime, err := u.GetPendingRecycleTime()
	if err != nil {
		return false
	}
	slog.Info("pending recycle time", "pendingTime", pendingTime, "now", time.Now())
	return time.Now().After(pendingTime)
}
