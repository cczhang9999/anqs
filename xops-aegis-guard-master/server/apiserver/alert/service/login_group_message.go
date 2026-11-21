package service

import (
	"net"
	"strconv"
	"strings"
	"time"

	alertmodel "xops-aegis-guard/server/apiserver/alert/model"
	devicemodel "xops-aegis-guard/server/apiserver/device/model"
)

func isRootLoginAlert(title string) bool {
	title = strings.ToLower(title)
	return strings.Contains(title, "r176") || strings.Contains(title, "r177") ||
		(strings.Contains(title, "root") &&
			(strings.Contains(title, "登录") || strings.Contains(title, "登陆") || strings.Contains(title, "login")))
}

func isLoginGroupAlert(row *alertmodel.SecurityAlert) bool {
	if row.AlertType != "suspicious_event" && row.AlertType != "custom_incident" {
		return false
	}
	return isRootLoginAlert(row.Title) || strings.Contains(row.Title, "异常登录") ||
		strings.Contains(row.Title, "异常登陆") || strings.Contains(row.Title, "非常用IP登录")
}

func loginGroupMessage(row *alertmodel.SecurityAlert, device *devicemodel.Device, by, note string, confirmedAt *time.Time) string {
	payload := jsonObject(row.LastPayload)
	if row.AlertType == "custom_incident" {
		payload = siemLoginPayload(payload, row.Title)
	}
	legacy := loginSnapshotFields(row.LastMessage)
	message := buildLoginAlertMessage(loginAlertInfo{
		title:      row.Title,
		severity:   coreSeverityText(row.Severity),
		providerID: row.ProviderAlertID,
		alertTime:  firstNonEmptyMessage(timeText(row.FirstProviderTime), timeText(row.LastProviderTime)),
	}, payload, legacy, device)
	if confirmedAt == nil {
		return message
	}
	return message + "\n\n【确认记录】\n" + strings.Join([]string{
		loginMessageField("确认人", by),
		loginMessageField("确认时间", confirmedAt.Format(time.DateTime)),
		loginMessageField("确认信息", note),
	}, "\n")
}

type loginAlertInfo struct {
	title      string
	severity   string
	providerID string
	alertTime  string
	assetName  string
	privateIP  string
}

func buildLoginAlertMessage(info loginAlertInfo, payload, legacy map[string]any, device *devicemodel.Device) string {
	value := func(aliases ...string) string {
		return firstNonEmptyMessage(directValue(payload, aliases...), loginDetailValue(payload, aliases...), directValue(legacy, aliases...))
	}
	assetName := firstNonEmptyMessage(info.assetName, value("InstanceName", "instance_name", "HostName", "host_name", "AssetName", "资产名称", "实例名称", "资产"))
	privateIP := firstNonEmptyMessage(info.privateIP, value("IntranetIp", "PrivateIP", "private_ip", "host_ip", "私网 IP", "内网IP", "ip"))
	if device != nil {
		assetName = firstNonEmptyMessage(assetName, device.InstanceName)
		privateIP = firstNonEmptyMessage(privateIP, device.PrivateIP)
	}
	return strings.Join([]string{
		loginMessageField("告警时间", loginTime(firstNonEmptyMessage(
			value("OccurrenceTime", "OccurrenceTimeStamp", "AlertTime", "alert_time", "FirstTime", "CreateTime", "告警时间", "首次发生时间"), info.alertTime))),
		loginMessageField("日志时间", loginTime(value("LogTimeStamp", "log_timestamp", "LogTime", "log_time", "日志时间戳", "日志时间", "登录时间"))),
		loginMessageField("告警名", info.title),
		loginMessageField("事件 ID", firstNonEmptyMessage(value("SecurityEventIds", "SecurityEventID", "EventID", "event_id", "事件 ID", "事件ID"), info.providerID)),
		loginMessageField("等级标签", info.severity),
		"",
		"【受影响资产】",
		loginMessageField("资产名称", assetName),
		loginMessageField("私网 IP", privateIP),
		"",
		"【登录详情】",
		loginMessageField("username", value("username", "user_name", "登录账号", "用户名", "用户")),
		loginMessageField("login_type", value("login_type", "LoginType", "登录方式", "登录协议")),
		loginMessageField("dst_ip", value("dst_ip", "dest_ip", "DstIp", "目标IP")),
		loginMessageField("dst_port", value("dst_port", "dest_port", "DstPort", "目标端口")),
		loginMessageField("src_ip", firstNonEmptyMessage(value("src_ip", "SrcIp", "登录源IP", "登录来源IP", "来源IP"), loginLocationSourceIP(payload))),
		loginMessageField("net_connect_dir", value("net_connect_dir", "NetConnectDir", "连接方向")),
	}, "\n")
}

// Keep each Cloud SIEM login's fields together. An incident may contain
// multiple alerts, so do not fill gaps with another login's values.
func siemLoginPayload(payload map[string]any, title string) map[string]any {
	var selected map[string]any
	for _, alert := range jsonObjectList(payload["AlertDetails"]) {
		detail := jsonObject(alert["AlertDetail"])
		standard := jsonObject(detail["ctdr_standard_alert"])
		if standard == nil {
			standard = jsonObject(alert["ctdr_standard_alert"])
		}
		fields := make(map[string]any)
		mergeMap(fields, alert)
		mergeMap(fields, detail)
		mergeMap(fields, jsonObject(alert["ExtendContent"]))
		mergeMap(fields, standard)
		mergeMap(fields, jsonObject(standard["extend_content"]))
		mergeMap(fields, jsonObject(fields["extend_content"]))
		alertName := directValue(fields, "AlertName", "alert_name", "Name")
		if strings.EqualFold(alertName, title) || sameRootLoginRule(alertName, title) {
			selected = fields
			break
		}
		if selected == nil && !isRootLoginAlert(alertName) && (directValue(fields, "login_type", "LoginType") != "" ||
			directValue(fields, "username", "user_name") != "" && directValue(fields, "src_ip", "dst_ip") != "") {
			selected = fields
		}
	}
	fields := make(map[string]any)
	mergeMap(fields, payload)
	mergeMap(fields, selected)
	for _, asset := range mapSlice(payload["Assets"]) {
		name := incidentAssetName(asset)
		ip := incidentDisplayValue(asset, "IntranetIp", "PrivateIP", "private_ip", "host_ip")
		if name == "" && ip == "" {
			continue
		}
		if directValue(fields, "InstanceName", "instance_name", "HostName", "host_name", "AssetName") == "" {
			fields["InstanceName"] = name
		}
		if directValue(fields, "IntranetIp", "PrivateIP", "private_ip", "host_ip") == "" {
			fields["IntranetIp"] = ip
		}
		break
	}
	return fields
}

func sameRootLoginRule(a, b string) bool {
	a, b = strings.ToLower(a), strings.ToLower(b)
	for _, rule := range []string{"r176", "r177"} {
		if strings.Contains(a, rule) && strings.Contains(b, rule) {
			return true
		}
	}
	return false
}

func loginDetailValue(payload map[string]any, aliases ...string) string {
	for _, detail := range mapSlice(payload["Details"]) {
		for _, alias := range aliases {
			if strings.EqualFold(messageValue(detail["Name"]), alias) || strings.EqualFold(messageValue(detail["NameDisplay"]), alias) {
				if value := firstNonEmptyMessage(messageValue(detail["ValueDisplay"]), messageValue(detail["Value"])); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

func loginLocationSourceIP(payload map[string]any) string {
	value := loginDetailValue(payload, "location")
	if net.ParseIP(value) != nil {
		return value
	}
	return ""
}

func directValue(payload map[string]any, keys ...string) string {
	for _, candidate := range keys {
		for key, value := range payload {
			if strings.EqualFold(key, candidate) {
				if text := messageValue(value); text != "—" && text != "未提供" && !isMissingMessageValue(text) {
					return text
				}
			}
		}
	}
	return ""
}

func loginSnapshotFields(message string) map[string]any {
	fields := make(map[string]any)
	for _, line := range strings.Split(stripMarkdownBold(message), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "--"))
		label, value, found := strings.Cut(line, "：")
		if !found {
			label, value, found = strings.Cut(line, ": ")
		}
		if found {
			if icon, text, ok := strings.Cut(label, " "); ok && !strings.ContainsAny(icon, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
				label = text
			}
			fields[strings.TrimSpace(label)] = strings.TrimSpace(value)
		}
	}
	return fields
}

func loginMessageField(label, value string) string {
	value = strings.Join(strings.Fields(strings.ReplaceAll(value, "@", "＠")), " ")
	if isMissingMessageValue(value) {
		value = "—"
	}
	return label + "：" + value
}

func loginTime(value string) string {
	if timestamp, err := strconv.ParseInt(value, 10, 64); err == nil && timestamp > 0 {
		return formatMessageTime(timestamp)
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05Z07:00", "2006-01-02T15:04:05", time.DateTime} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format(time.DateTime)
		}
	}
	return value
}
