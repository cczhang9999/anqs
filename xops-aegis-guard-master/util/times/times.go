package times

import (
	"fmt"
	"strings"
	"time"
)

type ISOTime time.Time

const customLayout = "2006-01-02 15:04:05"

func (ct *ISOTime) UnmarshalJSON(b []byte) error {
	// 去掉引号
	str := strings.Trim(string(b), `"`)
	if str == "" {
		return nil
	}
	t, err := time.ParseInLocation(customLayout, str, time.Local)
	if err != nil {
		return err
	}
	*ct = ISOTime(t)
	return nil
}
func (t *ISOTime) MarshalJSON() ([]byte, error) {
	if time.Time(*t).IsZero() {
		return []byte(fmt.Sprintf("%q", "")), nil
	}

	// 注意：序列化成的时间字符串必须包含在双引号当中
	return []byte(fmt.Sprintf("%q", time.Time(*t).Format(time.DateTime))), nil
}

func GetFirstDayOfMonth() time.Time {
	now := time.Now()
	firstDay := time.Date(now.Year(), now.Month(), 1, 0, 0, 1, 0, now.Location())
	return firstDay
}
