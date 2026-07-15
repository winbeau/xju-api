package service

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// xju-api:new — ConsumeInviteCodeForRegistration 的消费/回滚协议测试
// (REFACTOR-PLAN §5.2 Register 收口,单测先行)。风险点:邀请码泄漏
// (消费后未回滚)与永久占用(回滚把已归属的码复活),两者都在此锁死。

func seedInviteCode(t *testing.T, code string, status int, expiredTime int64) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.InviteCode{
		Code:        code,
		Status:      status,
		CreatorId:   1,
		CreatedTime: common.GetTimestamp(),
		ExpiredTime: expiredTime,
	}).Error)
}

func fetchInviteCode(t *testing.T, code string) *model.InviteCode {
	t.Helper()
	ic := &model.InviteCode{}
	require.NoError(t, model.DB.Where("code = ?", code).First(ic).Error)
	return ic
}

func withInviteCodeRequired(t *testing.T, required bool) {
	t.Helper()
	prev := common.InviteCodeRequired
	common.InviteCodeRequired = required
	t.Cleanup(func() { common.InviteCodeRequired = prev })
}

func TestConsumeInviteCodeForRegistration_RequiredOff(t *testing.T) {
	withInviteCodeRequired(t, false)
	seedInviteCode(t, "off-code-000000000000000000000", common.InviteCodeStatusEnabled, 0)

	release, commit, err := ConsumeInviteCodeForRegistration("off-code-000000000000000000000")
	require.NoError(t, err)
	require.NotNil(t, release)
	require.NotNil(t, commit)

	// 开关关闭 = 完全不触碰邀请码表;release/commit 都必须是安全 no-op。
	release()
	commit(42)
	ic := fetchInviteCode(t, "off-code-000000000000000000000")
	assert.Equal(t, common.InviteCodeStatusEnabled, ic.Status)
	assert.Equal(t, 0, ic.UsedUserId)
}

func TestConsumeInviteCodeForRegistration_ConsumeAndCommit(t *testing.T) {
	withInviteCodeRequired(t, true)
	seedInviteCode(t, "commit-code-000000000000000000", common.InviteCodeStatusEnabled, 0)

	release, commit, err := ConsumeInviteCodeForRegistration("commit-code-000000000000000000")
	require.NoError(t, err)

	// 消费即置 used,并发第二次注册不可能再拿到同一码。
	assert.Equal(t, common.InviteCodeStatusUsed, fetchInviteCode(t, "commit-code-000000000000000000").Status)

	commit(42)
	ic := fetchInviteCode(t, "commit-code-000000000000000000")
	assert.Equal(t, common.InviteCodeStatusUsed, ic.Status)
	assert.Equal(t, 42, ic.UsedUserId)

	// commit 之后 release 必须是 no-op —— 已归属的码绝不能被复活(永久占用风险的反面)。
	release()
	ic = fetchInviteCode(t, "commit-code-000000000000000000")
	assert.Equal(t, common.InviteCodeStatusUsed, ic.Status)
	assert.Equal(t, 42, ic.UsedUserId)
}

func TestConsumeInviteCodeForRegistration_ReleaseRollsBack(t *testing.T) {
	withInviteCodeRequired(t, true)
	seedInviteCode(t, "release-code-00000000000000000", common.InviteCodeStatusEnabled, 0)

	release, commit, err := ConsumeInviteCodeForRegistration("release-code-00000000000000000")
	require.NoError(t, err)

	// 注册失败 → release 把码放回池子(否则邀请码泄漏:没注册成却烧掉一张码)。
	release()
	ic := fetchInviteCode(t, "release-code-00000000000000000")
	assert.Equal(t, common.InviteCodeStatusEnabled, ic.Status)
	assert.Equal(t, 0, ic.UsedUserId)

	// release 幂等;release 之后 commit 也必须是 no-op(码已回池,不能再标 used_user_id)。
	release()
	commit(42)
	ic = fetchInviteCode(t, "release-code-00000000000000000")
	assert.Equal(t, common.InviteCodeStatusEnabled, ic.Status)
	assert.Equal(t, 0, ic.UsedUserId)

	// 回池后的码可再次被消费(复活语义)。
	_, _, err = ConsumeInviteCodeForRegistration("release-code-00000000000000000")
	require.NoError(t, err)
}

func TestConsumeInviteCodeForRegistration_RejectsUnusableCodes(t *testing.T) {
	withInviteCodeRequired(t, true)
	seedInviteCode(t, "used-code-00000000000000000000", common.InviteCodeStatusUsed, 0)
	seedInviteCode(t, "disabled-code-0000000000000000", common.InviteCodeStatusDisabled, 0)
	seedInviteCode(t, "expired-code-00000000000000000", common.InviteCodeStatusEnabled, common.GetTimestamp()-60)

	cases := []struct {
		name string
		code string
	}{
		{"empty", ""},
		{"unknown", "no-such-code"},
		{"already used", "used-code-00000000000000000000"},
		{"disabled", "disabled-code-0000000000000000"},
		{"expired", "expired-code-00000000000000000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := ConsumeInviteCodeForRegistration(tc.code)
			assert.Error(t, err)
		})
	}
	// 过期码被拒后状态不被改写。
	assert.Equal(t, common.InviteCodeStatusEnabled, fetchInviteCode(t, "expired-code-00000000000000000").Status)
}

func TestConsumeInviteCodeForRegistration_ConcurrentSingleUse(t *testing.T) {
	withInviteCodeRequired(t, true)
	seedInviteCode(t, "race-code-00000000000000000000", common.InviteCodeStatusEnabled, 0)

	const attempts = 8
	var wg sync.WaitGroup
	errs := make([]error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _, errs[idx] = ConsumeInviteCodeForRegistration("race-code-00000000000000000000")
		}(i)
	}
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
		}
	}
	// 状态 CAS 保证一码一用:并发注册恰有一个赢家。
	assert.Equal(t, 1, wins)
	assert.Equal(t, common.InviteCodeStatusUsed, fetchInviteCode(t, "race-code-00000000000000000000").Status)
}
