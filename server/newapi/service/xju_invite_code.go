package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// xju-api:new — 邀请码注册消费收口(REFACTOR-PLAN §5.2)。
//
// 注册请求提交的 aff_code 是双语义字段:上游 new-api 只把它当可选的推荐人归因
// (查不到就丢弃);xju-api 在 InviteCodeRequired 开启时把同一字段再当一次性
// 邀请码,注册必须原子消费。本函数是这条横切逻辑的唯一入口,controller 的
// Register 只留「调用 + defer release + 成功后 commit」。
//
// 返回的 release / commit 二选一、恰好其一生效(单 goroutine 使用,不做锁):
//
//	release — 注册失败时回滚消费,把码放回池子。幂等;commit 之后调用是 no-op,
//	          已归属的码绝不会被复活(防"永久占用"的反向风险)。
//	commit  — 注册成功后登记最终 user id,并使 release 永久失效。
//
// 开关关闭时三个返回值恒为 (no-op, no-op, nil),邀请码表完全不被触碰。
// 并发一码一用由 model.ConsumeInviteCode 的状态 CAS 保证;本层只保证调用协议,
// 最大风险(消费后未回滚 = 邀请码泄漏)由 defer release 兜底,回滚失败会记 SysError。
func ConsumeInviteCodeForRegistration(affCode string) (release func(), commit func(userID int), err error) {
	if !common.InviteCodeRequired {
		return func() {}, func(int) {}, nil
	}
	if err := model.ConsumeInviteCode(affCode, 0); err != nil {
		return nil, nil, err
	}
	settled := false
	release = func() {
		if settled {
			return
		}
		settled = true
		if err := model.ReleaseInviteCode(affCode); err != nil {
			common.SysError("invite code release failed, code may stay consumed: " + err.Error())
		}
	}
	commit = func(userID int) {
		if settled {
			return
		}
		settled = true
		if err := model.SetInviteCodeUser(affCode, userID); err != nil {
			common.SysError("invite code commit failed to record user id: " + err.Error())
		}
	}
	return release, commit, nil
}
