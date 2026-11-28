# xops-aegis-guard

独立的阿里云安全告警采集服务。第一期以观察数据为目标：每分钟拉取告警，脱敏后去重合并入库；机器人通知代码已预留，但默认关闭且当前采集链路不实际发送通知。

工程结构沿用 `xops-port-check`：`cmd/server` 负责启动，`config` 管理配置，`logger/zaplog` 管理日志，`pkg/db` 管理数据库，`server/apiserver/base/router` 统一注册 Gin 路由，告警、设备、部门分别放在 `server/apiserver/<业务>/controller`，机器人接口放在 `server/api`。

## 当前范围

- 安骑士 `DescribeSuspEvents`：标准主机安全告警，包含可由该接口返回的 Cloud SIEM 网络安全告警。
- 安骑士 `ListHoneypotAlarmEvents`：蜜罐告警。
- 安骑士 `ListFileProtectClientEvent`：核心文件告警，保留原项目的生产实例过滤并排除 `/usr/local/aegis/`。
- Cloud SIEM `ListIncidents`：未处理的自定义事件，过滤“自适应”事件。
- 安骑士设备同步：`DescribeCloudCenterInstances`，设备可关联部门。
- 部门管理：部门配置 `robot_id`；通知请求使用 `robot-new` 的 `im_id`、`source_name`、`text` 字段，不发送 token，不使用 `im_name`，不保存通知记录。

## 去重和持续告警

`security_alerts` 是当前告警的合并视图，不是一条轮询一条记录。唯一键为：

`source + product_type + alert_type + fingerprint`

指纹优先使用阿里云稳定告警 ID；没有稳定 ID 时使用设备、告警标题和脱敏后的告警内容计算 SHA-256。重复拉取只更新最近时间、最近内容和累计次数；当上游时间或内容发生变化时累计 `occurrence_count`。部门名称会保存到 `department_snapshot`，后续移动设备不会改写历史告警的部门归属。

## 接口

- `GET /health`
- 告警：`POST/GET/GET :id/PUT/DELETE /v1/alerts`
- `GET /v1/alerts?source=&alert_type=&status=&severity=&department_id=&device_id=&limit=&offset=`
- `POST /v1/alerts/collect`：手动触发一次采集
- 设备：`POST/GET/GET :id/PUT/DELETE /v1/devices`
- `POST /v1/devices/sync`
- `PUT /v1/devices/:id/department`，请求体 `{ "department_id": 1 }`
- 部门：`POST/GET/GET :id/PUT/DELETE /v1/departments`

## 二期：自动分组规则

主机从安骑士同步进来后先进入「未分配」，再由分组规则按主机名自动归属到部门。规则按顺序执行，命中第一条即停止，只处理未分配主机，已分配（手动或规则）的主机不会被改写。

- 运算符：`contains` 包含 / `not_contains` 不包含 / `prefix` 前缀是 / `not_prefix` 前缀不是 / `suffix` 后缀是 / `not_suffix` 后缀不是 / `equals` 完全等于 / `regex` 正则匹配；全部区分大小写。
- 条件之间是「且」；一个条件里用中英文逗号分隔多个值，正值之间是「或」，否定类表示这些值都不能出现；正则表达式整体不按逗号拆分，必须能编译通过。
- 归属来源会随主机保存：`assign_source=rule` 记录规则 ID 与规则名称快照（规则删除后仍显示「规则：xxx（已删除）」），`assign_source=manual` 记录操作人；把主机改回「未分配」即交还给规则，下次执行时重新匹配。
- 规则接口：`GET/POST/PUT/DELETE /v1/group-rules|group-rule`、`PUT /v1/group-rule-enabled`、`PUT /v1/group-rule-order`（`top/up/down`）、`GET /v1/group-rulestats`、`GET /v1/group-rule-preview`（试运行，不写库）、`POST /v1/group-rule-execute`（确认后一次性写入）。
- 设备接口新增 `source=rule|manual|unassigned` 筛选，响应包含 `assign_source`、`assign_rule_name`、`assign_rule_deleted`、`assign_operator`、`assigned_at`；`PUT /v1/device-departments` 传 `department_id=0` 表示批量交还给规则。
- 每次设备同步完成后执行分组规则，新同步或仍未分配的主机无需人工执行即可归属；手动归属保持不变。规则已有归属会按当前条件复核，失效时清理后重新匹配。
- 分组规则可通过 `GET /v1/group-rule-preview` 预览，确认后调用 `POST /v1/group-rule-execute` 立即执行；保存规则后也会在下一次设备同步时生效。
- 部门下仍被规则引用时禁止删除部门，避免规则指向不存在的目标部门。

## 启动

1. 修改 `config/config.yml` 中 MySQL、阿里云 AK/SK 和账号类型。
2. 阿里云 AK/SK 按原项目方式使用 `util/aes` 内置 key/nonce 加密后写入 YAML，配置文件不保存 AES 配置。
3. 启动 API 服务和内置任务：`go run ./cmd -f ./config/config.yml`。
4. 服务启动时自动执行 GORM 迁移，也可以先执行 `sql/schema.sql` 创建数据库。

`robot.enabled` 保持 `false` 即为观察模式。服务启动后由 `server/task` 内置调度器执行：告警每分钟拉取，设备每五分钟同步；告警增量游标和任务锁存储在 Redis，所有原始 payload 和生成的消息在落库前都会经过 tracing-logs 同源脱敏器。
