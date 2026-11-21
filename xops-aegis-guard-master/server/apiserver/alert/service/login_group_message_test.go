package service

import (
	"strings"
	"testing"
	"time"

	alertmodel "xops-aegis-guard/server/apiserver/alert/model"
	devicemodel "xops-aegis-guard/server/apiserver/device/model"
)

func TestLoginAlertGroupMessage(t *testing.T) {
	const title = "R177 root用户登陆失败"
	tests := []struct {
		name    string
		kind    string
		payload string
	}{
		{
			name: "host security detail fields",
			kind: "suspicious_event",
			payload: `{
				"OccurrenceTime": "2026-11-09 17:55:36",
				"Details": [
					{"NameDisplay":"日志时间戳","Value":"2026-11-09T17:55:58+08:00"},
					{"Name":"username","NameDisplay":"账号","Value":"root"},
					{"NameDisplay":"登录协议","ValueDisplay":"SSH"},
					{"NameDisplay":"目标IP","Value":"192.0.2.10"},
					{"NameDisplay":"目标端口","Value":22},
					{"NameDisplay":"location","Value":"192.0.2.15"},
					{"NameDisplay":"连接方向","Value":"in"}
				]
			}`,
		},
		{
			name: "SIEM matching alert with nested JSON",
			kind: "custom_incident",
			payload: `{
				"AlertDetails": [
					{"AlertName":"R176 root用户登陆成功","ExtendContent":{"username":"other","dst_port":2200}},
					{"AlertName":"R177 登录失败","AlertTime":"2026-11-09T17:55:36+08:00",
					 "AlertDetail":{"ctdr_standard_alert":{"extend_content":"{\"log_time\":\"2026-11-09 17:55:58\",\"username\":\"root\",\"login_type\":\"SSH\",\"dst_ip\":\"192.0.2.10\",\"dst_port\":22,\"src_ip\":\"192.0.2.15\",\"net_connect_dir\":\"in\"}"}}}
				]
			}`,
		},
	}
	want := `告警时间：2026-11-09 17:55:36
日志时间：2026-11-09 17:55:58
告警名：R177 root用户登陆失败
事件 ID：285c347f-cc73-4c04-802f-d7a4c6863b25
等级标签：中危

【受影响资产】
资产名称：app-server-01
私网 IP：192.0.2.10

【登录详情】
username：root
login_type：SSH
dst_ip：192.0.2.10
dst_port：22
src_ip：192.0.2.15
net_connect_dir：in`
	svc := &Service{}
	device := &devicemodel.Device{InstanceName: "app-server-01", PrivateIP: "192.0.2.10"}
	at := time.Date(2026, 11, 21, 15, 53, 47, 0, time.FixedZone("UTC+8", 8*3600))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			row := &alertmodel.SecurityAlert{Title: title, AlertType: tc.kind, LastPayload: tc.payload,
				Severity: "medium", ProviderAlertID: "285c347f-cc73-4c04-802f-d7a4c6863b25",
				ConfirmedBy: "另一个确认人", ConfirmNote: "另一条确认记录"}
			if got := svc.alertGroupMessage(row, device, "", "", nil); got != want {
				t.Fatalf("group message:\n%s\nwant:\n%s", got, want)
			}
			wantConfirmed := want + "\n\n【确认记录】\n确认人：值班人员\n确认时间：2026-11-21 15:53:47\n确认信息：已核实登录记录"
			if got := svc.alertGroupMessage(row, device, "值班人员", "已核实登录记录", &at); got != wantConfirmed {
				t.Fatalf("confirmed message:\n%s\nwant:\n%s", got, wantConfirmed)
			}
		})
	}
}

func TestLoginAlertGroupMessageLegacySnapshot(t *testing.T) {
	svc := &Service{}
	row := &alertmodel.SecurityAlert{
		Title: "异常登录", AlertType: "suspicious_event", ProviderAlertID: "42", Severity: "suspicious",
		LastMessage: "⏱ 首次发生时间：2026-11-09 17:55:36\n🖥 实例名称：app-server-01\n🔐 登录协议：SSH\n👤 登录账号：tester",
	}
	got := svc.alertGroupMessage(row, nil, "", "", nil)
	for _, field := range []string{"告警时间：2026-11-09 17:55:36", "日志时间：—", "资产名称：app-server-01", "login_type：SSH", "username：tester", "src_ip：—"} {
		if !strings.Contains(got, field) {
			t.Errorf("group message missing %q:\n%s", field, got)
		}
	}
	row.AlertType = "core_file"
	if got := svc.alertGroupMessage(row, nil, "", "", nil); got != row.LastMessage {
		t.Fatalf("non-login notification changed: %q", got)
	}
}
