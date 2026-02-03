package parse

import "time"

// CalcNextResetTime 计算下一次流量重置时间的 Unix 时间戳
// flowResetTime: 0 表示不重置，1-31 表示每月第 N 天重置
// now: 当前时间
// 返回: Unix 时间戳（秒），flowResetTime=0 时返回 0
func CalcNextResetTime(flowResetTime int, now time.Time) int64 {
	if flowResetTime == 0 {
		return 0
	}

	year, month, day := now.Date()
	loc := now.Location()

	// 本月的重置日
	resetDay := flowResetTime
	lastDayOfMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	if resetDay > lastDayOfMonth {
		resetDay = lastDayOfMonth
	}

	if day < resetDay {
		// 今天在重置日之前，下次重置是本月
		return time.Date(year, month, resetDay, 0, 0, 0, 0, loc).Unix()
	}

	// 今天 >= 重置日，下次重置是下个月
	nextMonth := month + 1
	nextYear := year
	if nextMonth > 12 {
		nextMonth = 1
		nextYear++
	}

	lastDayOfNextMonth := time.Date(nextYear, nextMonth+1, 0, 0, 0, 0, 0, loc).Day()
	resetDay = flowResetTime
	if resetDay > lastDayOfNextMonth {
		resetDay = lastDayOfNextMonth
	}

	return time.Date(nextYear, nextMonth, resetDay, 0, 0, 0, 0, loc).Unix()
}
