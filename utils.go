package abe

import (
	"time"

	"github.com/hako/durafmt"
)

// FormatDuration 将 time.Duration 格式化为中文显示
// 例如: 15 分钟, 1 小时 30 分钟
func FormatDuration(d time.Duration) string {
	zhUnits, _ := durafmt.DefaultUnitsCoder.Decode("年:年,周:周,天:天,小时:小时,分钟:分钟,秒:秒,毫秒:毫秒,微秒:微秒")
	return durafmt.Parse(d).LimitFirstN(2).Format(zhUnits)
}
