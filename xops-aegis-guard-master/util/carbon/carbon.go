package carbon

import "time"

// Carbon 提供旧版辅助方法所需的时间处理功能。
type Carbon = time.Time

func GetNextSecond(startTime string) Carbon { return Parse(startTime).Add(time.Second) }
func GetNow() Carbon                        { return time.Now() }
func GetCarbonTime(startTime string) Carbon { return Parse(startTime) }
func Parse(value string) time.Time          { t, _ := time.Parse(time.RFC3339, value); return t }
