#!/usr/bin/env python3
"""模拟 R177 root用户登陆失败 告警，验证 Telegram 推送链路。

用法：
    ./test-r177-telegram.py           # 写入告警并确认，推送 Telegram
    ./test-r177-telegram.py --create  # 只写入，不确认（不推送）
    ./test-r177-telegram.py --confirm-only <alert_id>  # 只确认已有告警

依赖：后端已启动在 127.0.0.1:30044，且 config/config.yml 已启用 telegram。
"""

import json
import os
import sys
import time
import urllib.request
import urllib.error

API_BASE = os.environ.get("API_BASE", "http://127.0.0.1:30044/v1")
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))

# 基于 alert-6244-db-record.txt 中真实的 R177 告警数据构造
# alert_type=custom_incident + title 含 "R177" → 走 loginGroupMessage 格式
PROVIDER_ALERT_ID = "test-r177-root-login-{}".format(int(time.time()))
FINGERPRINT = "test-r177-root-login:{}".format(int(time.time()))

# Cloud SIEM incident payload — 包含 AlertDetails 中的登录字段
# loginGroupMessage 会从 AlertDetails 提取 username/login_type/src_ip 等
PAYLOAD = {
    "AlarmEventName": "login_common_ip",
    "AlarmEventNameDisplay": "R177 root用户登陆失败",
    "AlarmEventType": "异常登录",
    "AlarmEventTypeDisplay": "异常登录",
    "AlarmUniqueInfo": "059d8edc1584598a062d9947aa108399",
    "DataSource": "aegis_login_log",
    "Desc": "检测到 root 账号通过 SSH 登录目标主机失败。root 账号具备系统最高权限，"
            "针对 root 的登录失败可能由账号密码输入错误、自动化脚本配置异常或未经授权的登录尝试导致。",
    "Details": [
        {
            "NameDisplay": "提示",
            "Type": "html",
            "Value": "root 账号通过 SSH 登录失败",
            "ValueDisplay": "root 账号通过 SSH 登录失败"
        },
        {
            "NameDisplay": "登录时间",
            "Type": "text",
            "Value": "2026-11-21 18:00:00",
            "ValueDisplay": "2026-11-21 18:00:00"
        },
        {
            "NameDisplay": "登录账号",
            "Type": "text",
            "Value": "root",
            "ValueDisplay": "root"
        },
        {
            "NameDisplay": "登录协议",
            "Type": "text",
            "Value": "SSH",
            "ValueDisplay": "SSH"
        },
        {
            "NameDisplay": "登录源IP",
            "Type": "text",
            "Value": "10.109.248.208",
            "ValueDisplay": "10.109.248.208"
        },
        {
            "NameDisplay": "登录地",
            "Type": "text",
            "Value": "局域网",
            "ValueDisplay": "局域网"
        },
        {
            "NameDisplay": "描述",
            "Type": "text",
            "Value": "检测到 root 账号通过 SSH 登录目标主机失败。root 账号具备系统最高权限，"
                    "针对 root 的登录失败可能由账号密码输入错误、自动化脚本配置异常或未经授权的登录尝试导致。",
            "ValueDisplay": "检测到 root 账号通过 SSH 登录目标主机失败。"
        }
    ],
    "EventStatus": 8,
    "EventSubType": "login_common_ip",
    "Id": 700476004,
    "InstanceId": "VMware-42 2f e3 b6 3e 62 15 23-78 1a 41 ac 03 e0 b7 2d",
    "InstanceName": "Z-JP-GL-IDC-JumpServer01",
    "IntranetIp": "10.109.248.208",
    "LastTime": "2026-11-21 18:00:00",
    "Level": "medium",
    "Name": "R177 root用户登陆失败",
    "OccurrenceTime": "2026-11-21 18:00:00",
    "SecurityEventIds": "700476004",
    "Stages": "[\"凭据窃取\"]",
    "TacticItems": [
        {"TacticDisplayName": "凭据窃取", "TacticId": "TA0006"}
    ],
    "UniqueInfo": "62f6c025ac41595d2ad23d97977aae88",
    "Uuid": "inet-ff2e2c6e-b85f-41c6-a808-b4952ed0bae4"
}

# last_message: 简化的 Cloud SIEM incident 格式（custom_incident）
# 确认推送时，loginGroupMessage 会从 payload 重新构建消息，不用 last_message
MESSAGE = """📑 告警名称：R177 root用户登陆失败
🆔 告警编号：{provider_id}
🚨 告警等级：中危
⏱ 告警时间：2026-11-21 18:00:00
☁ 告警来源：企业版
🏷 内容：
--资产：Z-JP-GL-IDC-JumpServer01
--ip：10.109.248.208
--用户：root
--描述：检测到 root 账号通过 SSH 登录目标主机失败。root 账号具备系统最高权限，针对 root 的登录失败可能由账号密码输入错误、自动化脚本配置异常或未经授权的登录尝试导致。""".format(provider_id=PROVIDER_ALERT_ID)


def request(method, path, payload=None):
    """发起一次 JSON 请求，返回 (code, body_dict)。"""
    data = None if payload is None else json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(
        API_BASE + path,
        data=data,
        method=method,
        headers={"Content-Type": "application/json"},
    )
    try:
        with OPENER.open(req, timeout=30) as resp:
            return resp.status, json.loads(resp.read().decode("utf-8"))
    except urllib.error.HTTPError as exc:
        return exc.code, {"raw": exc.read().decode("utf-8", "replace")}
    except Exception as exc:
        return 0, {"error": str(exc)}


def create_alert():
    """创建 R177 root登录失败告警。"""
    alert = {
        "source": "aliyun",
        "product_type": "AQS",
        "alert_type": "custom_incident",
        "provider_alert_id": PROVIDER_ALERT_ID,
        "fingerprint": FINGERPRINT,
        "title": "R177 root用户登陆失败",
        "severity": "medium",
        "status": "firing",
        "device_id": 0,
        "last_payload": json.dumps(PAYLOAD, ensure_ascii=False),
        "last_message": MESSAGE,
    }
    print("创建告警：R177 root用户登陆失败")
    print(f"  provider_alert_id: {PROVIDER_ALERT_ID}")
    print(f"  fingerprint: {FINGERPRINT}")
    status, body = request("POST", "/alert", alert)
    if status == 200 and body.get("code") == 200:
        data = body.get("data") or {}
        alert_id = (data.get("TimeModel") or {}).get("id") or data.get("id")
        print(f"  [OK] 告警已创建，id={alert_id}")
        return alert_id
    print(f"  [FAIL] status={status} body={str(body)[:300]}")
    return None


def confirm_alert(alert_id):
    """确认告警，触发 Telegram 推送。"""
    print(f"\n确认告警 id={alert_id}（触发 Telegram 推送）...")
    status, body = request("POST", "/alert-confirm", {
        "id": alert_id,
        "note": "测试确认：R177 root用户登陆失败 - 验证 Telegram 群推送",
    })
    if status != 200:
        print(f"  [FAIL] status={status} body={str(body)[:300]}")
        return False
    data = body.get("data") or {}
    sent = data.get("notification_sent")
    err = data.get("notification_error") or "-"
    title = data.get("title", "?")
    severity = data.get("severity", "?")
    status_val = data.get("status", "?")
    print(f"  告警标题: {title}")
    print(f"  告警等级: {severity}")
    print(f"  告警状态: {status_val}")
    print(f"  Telegram 推送: sent={sent}")
    print(f"  推送错误信息: {err}")
    if sent:
        print(f"\n  ✅ Telegram 群消息推送成功！请到 Telegram 群查看消息。")
        return True
    else:
        print(f"\n  ❌ Telegram 推送失败：{err}")
        return False


def main():
    args = sys.argv[1:]

    if "--confirm-only" in args:
        idx = args.index("--confirm-only")
        if idx + 1 >= len(args):
            print("用法: --confirm-only <alert_id>")
            sys.exit(1)
        alert_id = int(args[idx + 1])
        confirm_alert(alert_id)
        return

    create_only = "--create" in args

    alert_id = create_alert()
    if alert_id is None:
        sys.exit(1)

    if create_only:
        print(f"\n告警已创建（--create 模式，不确认）。")
        print(f"手动确认推送：./test-r177-telegram.py --confirm-only {alert_id}")
        return

    confirm_alert(alert_id)

    # 最终统计
    status, body = request("GET", "/alertstats")
    stats = body.get("data", body)
    print(f"\n当前告警统计: {json.dumps(stats, ensure_ascii=False)}")


if __name__ == "__main__":
    main()
