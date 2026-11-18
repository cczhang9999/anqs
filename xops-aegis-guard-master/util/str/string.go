package str

import (
	"fmt"
	"strconv"
	"strings"

	"xops-aegis-guard/util/json"
)

// ToJSONString 转换数据为json string
func ToJSONString(data interface{}) string {
	bytes, _ := json.Marshal(data)
	return string(bytes)
}

func SchemaSplit(dsn, schema string) []string {
	var result []string
	var start int

	for _, it := range strings.Split(schema, "$") {
		if it == "" {
			continue
		}
		itIndex := strings.Index(dsn, it)
		result = append(result, dsn[:itIndex])
		start = itIndex + len(it)
		dsn = dsn[start:]
	}

	return result
}

func SliceToJSONString(nums []int) string {
	strs := make([]string, len(nums))
	for i, v := range nums {
		strs[i] = strconv.Itoa(v)
	}
	return strings.Join(strs, ",")
}
func ConvertToSliceOfInt(interfaceSlice []interface{}) ([]int, error) {
	var intSlice []int
	for _, v := range interfaceSlice {
		switch value := v.(type) {
		case int:
			intSlice = append(intSlice, value)
		case string:
			// 尝试将字符串转换为整数
			intValue, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("无法转换为整数: %v", v)
			}
			intSlice = append(intSlice, intValue)
		case float64:
			intSlice = append(intSlice, int(value))
		default:
			return nil, fmt.Errorf("不支持的类型: %T", v)
		}
	}

	return intSlice, nil
}
func ToBool(str string) bool {
	return str == "true"
}
