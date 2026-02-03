package parse

import (
	"testing"
	"time"
)

func TestCalcNextResetTime_NoReset(t *testing.T) {
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(0, now)
	if result != 0 {
		t.Errorf("Expected 0 for flowResetTime=0, got %d", result)
	}
}

func TestCalcNextResetTime_BeforeResetDay(t *testing.T) {
	// 今天 1月1日，重置日是2号 → 下次重置是 1月2日
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(2, now)

	expected := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d, got %d", expected, result)
	}
}

func TestCalcNextResetTime_OnResetDay(t *testing.T) {
	// 今天 1月2日，重置日是2号 → 下次重置是 2月2日
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(2, now)

	expected := time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d, got %d", expected, result)
	}
}

func TestCalcNextResetTime_AfterResetDay(t *testing.T) {
	// 今天 1月15日，重置日是2号 → 下次重置是 2月2日
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(2, now)

	expected := time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d, got %d", expected, result)
	}
}

func TestCalcNextResetTime_MonthEndBoundary(t *testing.T) {
	// 2月没有31号，应该回退到2月28日（非闰年）
	now := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(31, now)

	// 1月31日在1月15日之后，所以应该是1月31日
	expected := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d (Jan 31), got %d", expected, result)
	}
}

func TestCalcNextResetTime_FebruaryLeapYear(t *testing.T) {
	// 闰年2月，重置日31号 → 回退到2月29日
	now := time.Date(2024, 2, 1, 12, 0, 0, 0, time.UTC) // 2024是闰年
	result := CalcNextResetTime(31, now)

	expected := time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d (Feb 29), got %d", expected, result)
	}
}

func TestCalcNextResetTime_FebruaryNonLeapYear(t *testing.T) {
	// 非闰年2月，重置日31号 → 回退到2月28日
	now := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC) // 2025不是闰年
	result := CalcNextResetTime(31, now)

	expected := time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d (Feb 28), got %d", expected, result)
	}
}

func TestCalcNextResetTime_YearCrossover(t *testing.T) {
	// 12月15日，重置日是2号 → 下次重置是次年1月2日
	now := time.Date(2025, 12, 15, 12, 0, 0, 0, time.UTC)
	result := CalcNextResetTime(2, now)

	expected := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC).Unix()
	if result != expected {
		t.Errorf("Expected %d (2026-01-02), got %d", expected, result)
	}
}
