package tools

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"xops-aegis-guard/server/apiserver/grouprule/model"
)

// 匹配运算符。文本条件区分大小写，IP 条件比较设备内网 IP。
const (
	OperatorContains    = "contains"     // 包含
	OperatorNotContains = "not_contains" // 不包含
	OperatorPrefix      = "prefix"       // 前缀是
	OperatorNotPrefix   = "not_prefix"   // 前缀不是
	OperatorSuffix      = "suffix"       // 后缀是
	OperatorNotSuffix   = "not_suffix"   // 后缀不是
	OperatorEquals      = "equals"       // 完全等于
	OperatorRegex       = "regex"        // 正则匹配
	OperatorIPIn        = "ip_in"        // IP 属于
	OperatorIPNotIn     = "ip_not_in"    // IP 不属于
)

// OperatorLabels 与前端下拉保持一致，用于错误提示和日志。
var OperatorLabels = map[string]string{
	OperatorContains:    "包含",
	OperatorNotContains: "不包含",
	OperatorPrefix:      "前缀是",
	OperatorNotPrefix:   "前缀不是",
	OperatorSuffix:      "后缀是",
	OperatorNotSuffix:   "后缀不是",
	OperatorEquals:      "完全等于",
	OperatorRegex:       "正则匹配",
	OperatorIPIn:        "IP 属于",
	OperatorIPNotIn:     "IP 不属于",
}

// IsValidOperator 判断运算符是否受支持。
func IsValidOperator(operator string) bool {
	_, ok := OperatorLabels[operator]
	return ok
}

// IsNegativeOperator 判断是否为否定类运算符：多个值之间要求“都不能出现”。
func IsNegativeOperator(operator string) bool {
	switch operator {
	case OperatorNotContains, OperatorNotPrefix, OperatorNotSuffix, OperatorIPNotIn:
		return true
	default:
		return false
	}
}

// IsIPOperator 判断条件是否比较设备内网 IP。
func IsIPOperator(operator string) bool {
	return operator == OperatorIPIn || operator == OperatorIPNotIn
}

// NormalizeValues 按中英文逗号拆分条件值，去掉首尾空格与空值。
func NormalizeValues(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' })
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		values = append(values, value)
	}
	return values
}

// CompiledCondition 是预编译后的条件，正则只编译一次。
type CompiledCondition struct {
	Operator string
	Values   []string
	Regex    *regexp.Regexp
	Prefixes []netip.Prefix
}

// CompileConditions 校验并预编译条件列表；任何一条非法都会返回错误。
func CompileConditions(conditions []model.Condition) ([]CompiledCondition, error) {
	if len(conditions) == 0 {
		return nil, fmt.Errorf("至少需要一个匹配条件")
	}
	compiled := make([]CompiledCondition, 0, len(conditions))
	for index, condition := range conditions {
		operator := strings.TrimSpace(condition.Operator)
		if !IsValidOperator(operator) {
			return nil, fmt.Errorf("第%d个条件的运算符不受支持", index+1)
		}
		if operator == OperatorRegex {
			expression := strings.TrimSpace(condition.Value)
			if expression == "" {
				return nil, fmt.Errorf("第%d个条件的正则表达式不能为空", index+1)
			}
			re, err := regexp.Compile(expression)
			if err != nil {
				return nil, fmt.Errorf("第%d个条件的正则表达式格式错误", index+1)
			}
			compiled = append(compiled, CompiledCondition{Operator: operator, Regex: re})
			continue
		}
		values := NormalizeValues(condition.Value)
		if len(values) == 0 {
			return nil, fmt.Errorf("第%d个条件的值不能为空", index+1)
		}
		item := CompiledCondition{Operator: operator, Values: values}
		if IsIPOperator(operator) {
			for _, value := range values {
				prefix, err := parseIPValue(value)
				if err != nil {
					return nil, fmt.Errorf("第%d个条件的 IP 或 CIDR 网段格式错误: %s", index+1, value)
				}
				item.Prefixes = append(item.Prefixes, prefix)
			}
		}
		compiled = append(compiled, item)
	}
	return compiled, nil
}

// parseIPValue 将单个地址或 CIDR 网段统一为前缀，兼容 IPv4 和 IPv6。
func parseIPValue(value string) (netip.Prefix, error) {
	if strings.Contains(value, "/") {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return netip.Prefix{}, err
		}
		if prefix.Addr().Is4In6() && prefix.Bits() >= 96 {
			prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
		}
		return prefix.Masked(), nil
	}
	addr, err := netip.ParseAddr(value)
	if err != nil || addr.Zone() != "" {
		return netip.Prefix{}, fmt.Errorf("无效的 IP 地址")
	}
	addr = addr.Unmap()
	return netip.PrefixFrom(addr, addr.BitLen()), nil
}

// MatchIP 判断内网 IP 是否满足条件。缺失或非法 IP 不匹配否定条件。
func (c CompiledCondition) MatchIP(privateIP string) bool {
	if !IsIPOperator(c.Operator) || len(c.Prefixes) == 0 {
		return false
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(privateIP))
	if err != nil || addr.Zone() != "" {
		return false
	}
	matched := false
	for _, prefix := range c.Prefixes {
		if prefix.Contains(addr.Unmap()) || prefix.Contains(addr) {
			matched = true
			break
		}
	}
	if c.Operator == OperatorIPNotIn {
		return !matched
	}
	return matched
}

// Match 判断单个条件是否命中主机名。多个条件之间是“且”的关系。
func (c CompiledCondition) Match(hostname string) bool {
	switch c.Operator {
	case OperatorContains:
		return anyValue(c.Values, func(value string) bool { return strings.Contains(hostname, value) })
	case OperatorNotContains:
		return noValue(c.Values, func(value string) bool { return strings.Contains(hostname, value) })
	case OperatorPrefix:
		return anyValue(c.Values, func(value string) bool { return strings.HasPrefix(hostname, value) })
	case OperatorNotPrefix:
		return noValue(c.Values, func(value string) bool { return strings.HasPrefix(hostname, value) })
	case OperatorSuffix:
		return anyValue(c.Values, func(value string) bool { return strings.HasSuffix(hostname, value) })
	case OperatorNotSuffix:
		return noValue(c.Values, func(value string) bool { return strings.HasSuffix(hostname, value) })
	case OperatorEquals:
		return anyValue(c.Values, func(value string) bool { return hostname == value })
	case OperatorRegex:
		return c.Regex != nil && c.Regex.MatchString(hostname)
	default:
		return false
	}
}

// MatchAll 判断一组条件是否全部命中。
func MatchAll(conditions []CompiledCondition, hostname string) bool {
	return MatchDevice(conditions, hostname, "")
}

// MatchDevice 按条件选择主机名或内网 IP，所有条件之间为“且”。
// 纯 IP 规则允许设备没有主机名，文本条件仍要求非空主机名。
func MatchDevice(conditions []CompiledCondition, hostname, privateIP string) bool {
	for _, condition := range conditions {
		if IsIPOperator(condition.Operator) {
			if !condition.MatchIP(privateIP) {
				return false
			}
		} else if strings.TrimSpace(hostname) == "" || !condition.Match(hostname) {
			return false
		}
	}
	return len(conditions) > 0
}

func anyValue(values []string, test func(string) bool) bool {
	for _, value := range values {
		if test(value) {
			return true
		}
	}
	return false
}

// noValue 用于否定类条件：所有值都不能命中才算通过。
func noValue(values []string, test func(string) bool) bool {
	for _, value := range values {
		if test(value) {
			return false
		}
	}
	return true
}
