package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
	"xops-aegis-guard/desensitize"
	"xops-aegis-guard/logger"
	"xops-aegis-guard/pkg/aliyun"
	"xops-aegis-guard/pkg/redisstore"
	"xops-aegis-guard/pkg/user"
	robotapi "xops-aegis-guard/server/api"
	alertdao "xops-aegis-guard/server/apiserver/alert/dao"
	alertmodel "xops-aegis-guard/server/apiserver/alert/model"
	alerttools "xops-aegis-guard/server/apiserver/alert/tools"
	alertval "xops-aegis-guard/server/apiserver/alert/val"
	departmentdao "xops-aegis-guard/server/apiserver/department/dao"
	devicedao "xops-aegis-guard/server/apiserver/device/dao"
	devicemodel "xops-aegis-guard/server/apiserver/device/model"
	deviceservice "xops-aegis-guard/server/apiserver/device/service"
	jsonutil "xops-aegis-guard/util/json"
	"xops-aegis-guard/util/pager"

	"github.com/jinzhu/copier"
	"gorm.io/gorm"
)

type Service struct {
	clients  *aliyun.Clients
	masker   *desensitize.Masker
	cursor   *redisstore.Store
	notifier *robotapi.Notifier
}

const (
	alertPageSize                = 100
	alertMaxPages                = 0 // 0 means follow the provider total count without truncation.
	repeatedNotificationInterval = 30 * time.Minute
	incidentAssetRetryLimit      = 5
)

var instance *Service

func Init(clients *aliyun.Clients, masker *desensitize.Masker, notifiers ...*robotapi.Notifier) {
	var notifier *robotapi.Notifier
	if len(notifiers) > 0 {
		notifier = notifiers[0]
	}
	instance = &Service{clients: clients, masker: masker, cursor: redisstore.New(), notifier: notifier}
	deviceservice.Init(clients)
}

func CollectNow() error {
	if instance == nil {
		return errors.New("alert service is not initialized")
	}
	// The lock only prevents overlapping collectors; the short TTL also
	// allows a crashed process to recover quickly.
	release, err := instance.cursor.AcquireLock(10 * time.Second)
	if err != nil {
		return err
	}
	defer release()
	return instance.collectAll()
}

// Collect 是 HTTP 层使用的请求对象入口；CollectNow 保留给任务调度和内部调用。
func Collect(req *alertval.CollectAlertsReq) error { return CollectNow() }

func SyncDevices() error {
	return deviceservice.Sync()
}

func RebindUnassociated() error {
	if instance == nil {
		return errors.New("alert service is not initialized")
	}
	return instance.rebindUnassociatedAlerts()
}

func RefreshSparseMessages() error {
	if instance == nil {
		return errors.New("alert service is not initialized")
	}
	return instance.refreshSparseMessages()
}

func IsNotFound(err error) bool { return alertdao.IsNotFound(err) }

func Create(req *alertval.SaveAlertReq) error {
	row := &alertmodel.SecurityAlert{}
	if err := copier.Copy(row, &req.SecurityAlert); err != nil {
		return err
	}
	if row.LastSeenAt.IsZero() {
		row.LastSeenAt = time.Now()
	}
	if row.Status == "" {
		row.Status = "firing"
	}
	if row.OccurrenceCount == 0 {
		row.OccurrenceCount = 1
	}
	maskAlertContent(row)
	if err := alertdao.Save(row); err != nil {
		return err
	}
	req.SecurityAlert = *row
	return nil
}

func List(req *alertval.ListAlertReq) ([]alertval.AlertListItem, int64, int, int, error) {
	limit, offset := pager.Parse(strconv.Itoa(req.Limit), strconv.Itoa(req.Offset))
	scope, err := resolveAlertDeviceScope(alertDeviceScopeRequest{UserID: requestUserID(req.AuthUser), Host: req.Host, IP: req.IP, DepartmentID: req.DepartmentID, DeviceID: req.DeviceID})
	if err != nil {
		return nil, 0, limit, offset, err
	}
	keywordDeviceIDs := []uint(nil)
	if req.Keyword != "" {
		devices, deviceErr := devicedao.ListByIDs(scope.deviceIDs)
		if deviceErr != nil {
			return nil, 0, limit, offset, deviceErr
		}
		keywordDeviceIDs = matchingDeviceIDs(devices, req.Keyword)
	}
	rows, total, err := alertdao.List(alertdao.ListOptions{
		Limit:               limit,
		Offset:              offset,
		Source:              req.Source,
		AlertType:           req.AlertType,
		Status:              req.Status,
		Severity:            req.Severity,
		Keyword:             req.Keyword,
		KeywordDeviceIDs:    keywordDeviceIDs,
		From:                req.From,
		To:                  req.To,
		DeviceIDs:           scope.deviceIDs,
		IncludeUnassociated: scope.includeUnassociated,
	})
	if err != nil {
		return nil, total, limit, offset, err
	}
	items, err := toAlertListItems(rows)
	return items, total, limit, offset, err
}

func matchingDeviceIDs(devices []devicemodel.Device, keyword string) []uint {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return nil
	}
	ids := make([]uint, 0)
	for _, device := range devices {
		if strings.Contains(strings.ToLower(device.InstanceName), keyword) ||
			strings.Contains(strings.ToLower(device.InstanceID), keyword) ||
			strings.Contains(strings.ToLower(device.PrivateIP), keyword) {
			ids = append(ids, device.ID)
		}
	}
	return ids
}

func All(req *alertval.GetAlertsReq) ([]alertval.AlertListItem, error) {
	scope, err := resolveAlertDeviceScope(alertDeviceScopeRequest{UserID: requestUserID(req.AuthUser)})
	if err != nil {
		return nil, err
	}
	rows, err := alertdao.All(scope.deviceIDs, scope.includeUnassociated)
	if err != nil {
		return nil, err
	}
	return toAlertListItems(rows)
}

func Stats(req *alertval.GetAlertStatsReq) (total, pending, confirmed, today int64, severity map[string]int64, err error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrow := start.AddDate(0, 0, 1)
	scope, err := resolveAlertDeviceScope(alertDeviceScopeRequest{UserID: requestUserID(req.AuthUser)})
	if err != nil {
		return 0, 0, 0, 0, nil, err
	}
	return alertdao.Stats(start, tomorrow, scope.deviceIDs, scope.includeUnassociated)
}

func Get(req *alertval.GetAlertReq) (*alertval.AlertDetailResp, error) {
	row, err := alertdao.Get(req.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureAlertVisible(row, requestUserID(req.AuthUser)); err != nil {
		return nil, err
	}
	sanitizeAlertRow(row)
	return toAlertDetailResp(row), nil
}

func ListHistory(req *alertval.ListAlertHistoryReq) ([]alertval.AlertHistoryItem, error) {
	alert, err := alertdao.Get(req.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureAlertVisible(alert, requestUserID(req.AuthUser)); err != nil {
		return nil, err
	}
	if err := alertdao.ReconcileConfirmation(req.ID); err != nil {
		return nil, err
	}
	alert, err = alertdao.Get(req.ID)
	if err != nil {
		return nil, err
	}
	rows, err := alertdao.ListOccurrences(req.ID)
	if err != nil {
		return nil, err
	}
	items := make([]alertval.AlertHistoryItem, 0)
	for _, row := range rows {
		items = append(items, alertval.AlertHistoryItem{
			ID:          row.ID,
			AlertID:     row.AlertID,
			Title:       alert.Title,
			DeviceID:    alert.DeviceID,
			Severity:    alert.Severity,
			OccurredAt:  row.OccurredAt.Format("2006-01-02T15:04:05"),
			Status:      occurrenceStatus(row.ConfirmedAt),
			ConfirmedBy: row.ConfirmedBy,
			ConfirmedAt: row.ConfirmedAt,
			ConfirmNote: row.ConfirmNote,
			Snapshot:    row.Snapshot,
		})
	}
	if len(items) > 0 {
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].OccurredAt > items[j].OccurredAt
		})
		return items, nil
	}
	// 上线历史明细前，旧数据只有聚合计数和最近一次快照，无法凭空还原
	// 每次发生记录。返回一条带标记的快照，避免历史弹窗完全空白。
	occurredAt := alert.LastSeenAt.Format("2006-01-02T15:04:05")
	if alert.LastProviderTime != nil {
		occurredAt = alert.LastProviderTime.Format("2006-01-02T15:04:05")
	}
	return []alertval.AlertHistoryItem{{
		ID:          alert.ID,
		AlertID:     alert.ID,
		Title:       alert.Title,
		DeviceID:    alert.DeviceID,
		Severity:    alert.Severity,
		OccurredAt:  occurredAt,
		Snapshot:    true,
		Status:      alert.Status,
		ConfirmedBy: alert.ConfirmedBy,
		ConfirmedAt: alert.ConfirmedAt,
		ConfirmNote: alert.ConfirmNote,
	}}, nil
}

func Update(req *alertval.SaveAlertReq) (*alertval.AlertDetailResp, error) {
	if _, err := alertdao.Get(req.ID); err != nil {
		return nil, err
	}
	row := &alertmodel.SecurityAlert{}
	if err := copier.Copy(row, &req.SecurityAlert); err != nil {
		return nil, err
	}
	row.ID = req.ID
	maskAlertContent(row)
	if err := alertdao.Save(row); err != nil {
		return nil, err
	}
	return toAlertDetailResp(row), nil
}

func occurrenceStatus(at *time.Time) string {
	if at != nil {
		return "confirmed"
	}
	return "firing"
}

func confirmationActor(auth *user.AuthUser) (int, string) {
	if auth != nil {
		if name := strings.TrimSpace(auth.Username); name != "" {
			return auth.UserId, name
		}
		return auth.UserId, "内部调用"
	}
	return 0, "内部调用"
}

func Confirm(req *alertval.ConfirmAlertReq) (*alertval.AlertDetailResp, error) {
	row, err := alertdao.Get(req.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureAlertVisible(row, requestUserID(req.AuthUser)); err != nil {
		return nil, err
	}
	byID, by := confirmationActor(req.AuthUser)
	note, confirmedAt := strings.TrimSpace(req.Note), time.Now().Truncate(time.Second)
	changed, err := alertdao.ConfirmOccurrences([]uint{req.ID}, req.OccurrenceID, note, byID, by, confirmedAt)
	if err != nil {
		return nil, err
	}
	row, err = alertdao.Get(req.ID)
	if err != nil {
		return nil, err
	}
	sanitizeAlertRow(row)
	if len(changed) == 0 {
		logger.Infof("alert confirmation produced no new occurrence: alert_id=%d occurrence_id=%d status=%s confirmed_count=%d pending_count=%d; notification skipped", req.ID, req.OccurrenceID, row.Status, row.ConfirmedCount, row.PendingCount)
		// 没有新的明细被确认：可能是重复提交，也可能是旧数据导致
		// aggregate 状态与 occurrence 不一致。尝试刷新一次汇总，让
		// 前端状态正确；若修复后仍是 confirmed，则给出友好的未发送原因。
		if row.Status != "confirmed" && row.PendingCount == 0 && row.ConfirmedCount > 0 {
			if reconcileErr := alertdao.ReconcileConfirmation(req.ID); reconcileErr == nil {
				row, _ = alertdao.Get(req.ID)
				sanitizeAlertRow(row)
			}
		}
		resp := toAlertDetailResp(row)
		if row.Status == "confirmed" {
			resp.NotificationError = "该告警已确认，未重复发送通知"
		}
		return resp, nil
	}
	resp := toAlertDetailResp(row)
	if instance == nil {
		logger.Warnf("alert confirmation notification skipped: alert_id=%d reason=notification service is not initialized", row.ID)
		resp.NotificationError = "告警通知服务未初始化"
		return resp, nil
	}
	logger.Infof("alert confirmation notification dispatch: alert_id=%d occurrence_id=%d changed_occurrences=%d", row.ID, req.OccurrenceID, len(changed))
	sent, notifyErr := instance.notifyConfirmedMessage(row, by, note, confirmedAt)
	resp.NotificationSent = sent
	if notifyErr != nil {
		resp.NotificationError = notifyErr.Error()
	}
	return resp, nil
}

func ConfirmBatch(req *alertval.ConfirmAlertsReq) ([]alertval.AlertDetailResp, error) {
	// Check every aggregate's visibility before entering the atomic write.
	for _, id := range req.IDs {
		row, err := alertdao.Get(id)
		if err != nil {
			return nil, err
		}
		if err := ensureAlertVisible(row, requestUserID(req.AuthUser)); err != nil {
			return nil, err
		}
	}
	byID, by := confirmationActor(req.AuthUser)
	note, confirmedAt := strings.TrimSpace(req.Note), time.Now().Truncate(time.Second)
	changed, err := alertdao.ConfirmOccurrences(req.IDs, 0, note, byID, by, confirmedAt)
	if err != nil {
		return nil, err
	}
	results := make([]alertval.AlertDetailResp, 0, len(changed))
	for _, id := range changed {
		row, err := alertdao.Get(id)
		if err != nil {
			return nil, err
		}
		sanitizeAlertRow(row)
		resp := toAlertDetailResp(row)
		if instance == nil {
			resp.NotificationError = "告警通知服务未初始化"
		} else {
			resp.NotificationSent, err = instance.notifyConfirmedMessage(row, by, note, confirmedAt)
			if err != nil {
				resp.NotificationError = err.Error()
			}
		}
		results = append(results, *resp)
	}
	// 部分或全部告警可能因已确认而未进入 changed；对这类状态不一致的
	// 聚合再刷新一次，确保返回给前端的状态是准确的。
	for i := range results {
		if results[i].NotificationError == "" && results[i].Status != "confirmed" &&
			results[i].PendingCount == 0 && results[i].ConfirmedCount > 0 {
			if reconcileErr := alertdao.ReconcileConfirmation(results[i].ID); reconcileErr == nil {
				if refreshed, getErr := alertdao.Get(results[i].ID); getErr == nil {
					results[i].Status = refreshed.Status
					results[i].ConfirmedCount = refreshed.ConfirmedCount
					results[i].PendingCount = refreshed.PendingCount
					results[i].ConfirmedBy = refreshed.ConfirmedBy
					results[i].ConfirmedAt = refreshed.ConfirmedAt
				}
			}
		}
	}
	return results, nil
}

func (s *Service) rebindUnassociatedAlerts() error {
	rows, err := alertdao.ListUnassociated(10000)
	if err != nil {
		return err
	}
	bound := 0
	for _, row := range rows {
		payload := make(map[string]any)
		if err := jsonutil.Unmarshal([]byte(row.LastPayload), &payload); err != nil {
			continue
		}
		payload = expandMergeFieldList(payload)
		device, _ := devicedao.Find(row.ProductType,
			findValue(payload, "uuid", "Uuid", "UUID", "aliyun_uuid", "AliyunUUID"),
			findValue(payload, "instanceId", "InstanceId", "instance_id", "InstanceID"),
			findValue(payload, "instanceName", "InstanceName", "instance_name"),
			findValue(payload, "intranetIp", "IntranetIp", "private_ip", "PrivateIP", "dest_ip", "DestIp"))
		if device == nil && row.AlertType == "custom_incident" && row.ProviderAlertID != "" {
			assets, assetErr := s.clients.IncidentAssets(row.ProductType, row.ProviderAlertID)
			if assetErr != nil {
				logger.Warnf("backfill cloud siem incident assets failed: alert_id=%d incident_uuid=%s error=%v", row.ID, row.ProviderAlertID, assetErr)
			} else {
				for _, asset := range assets {
					if device = findDeviceByAsset(row.ProductType, asset); device != nil {
						break
					}
				}
			}
		}
		if device == nil {
			continue
		}
		if err := alertdao.BindDevice(row.ID, device.ID); err != nil {
			return err
		}
		bound++
	}
	if bound > 0 {
		logger.Infof("unassociated alerts rebound: count=%d", bound)
	}
	return nil
}

func (s *Service) refreshSparseMessages() error {
	rows, err := alertdao.ListSparseMessages(10000)
	if err != nil {
		return err
	}
	refreshed := 0
	for i := range rows {
		row := &rows[i]
		payload := make(map[string]any)
		if err := jsonutil.Unmarshal([]byte(row.LastPayload), &payload); err != nil {
			continue
		}
		if masked, ok := s.mask(payload).(map[string]any); ok {
			payload = masked
		}
		maskedPayload := marshal(payload)
		maskedMessage := s.maskMessage(row.LastMessage)
		if !needsMessageRefresh(row) && maskedPayload == row.LastPayload && maskedMessage == row.LastMessage {
			continue
		}
		var device *devicemodel.Device
		if row.DeviceID != 0 {
			device, _ = devicedao.Get(row.DeviceID)
		}
		var message string
		switch row.AlertType {
		case "suspicious_event":
			item := aliyun.SuspEvent{
				AccountType:    row.ProductType,
				ProviderID:     row.ProviderAlertID,
				UniqueInfo:     findValue(payload, "UniqueInfo", "AlarmUniqueInfo"),
				Title:          row.Title,
				Description:    findValue(payload, "Desc", "Description"),
				Level:          row.Severity,
				InstanceID:     findValue(payload, "InstanceId", "InstanceID"),
				InstanceName:   findValue(payload, "InstanceName", "HostName", "Hostname"),
				UUID:           findValue(payload, "Uuid", "UUID"),
				PrivateIP:      findValue(payload, "IntranetIp", "PrivateIP", "private_ip"),
				PublicIP:       findValue(payload, "InternetIp", "PublicIP", "public_ip"),
				OccurrenceTime: findValue(payload, "OccurrenceTime", "FirstTime"),
				LastTime:       findValue(payload, "LastTime", "LatestTime"),
			}
			message = buildSuspiciousAlertMessage(item, alertPayloadMap(payload), device)
		case "honeypot":
			payload = expandMergeFieldList(payload)
			if device == nil {
				device, _ = devicedao.Find(row.ProductType,
					findValue(payload, "uuid", "Uuid", "UUID", "aliyun_uuid", "AliyunUUID"),
					findValue(payload, "instanceId", "InstanceId", "instance_id", "InstanceID"),
					findValue(payload, "instanceName", "InstanceName", "instance_name"),
					findValue(payload, "dest_ip", "DestIp", "intranetIp", "IntranetIp", "private_ip", "PrivateIP"))
			}
			instanceName := findValue(payload, "instanceName", "InstanceName", "instance_name")
			instanceID := findValue(payload, "instanceId", "InstanceId", "instance_id", "InstanceID")
			privateIP := findValue(payload, "dest_ip", "DestIp", "intranetIp", "IntranetIp", "private_ip", "PrivateIP")
			if device != nil {
				instanceName = firstNonEmptyMessage(instanceName, device.InstanceName)
				instanceID = firstNonEmptyMessage(instanceID, device.InstanceID)
				privateIP = firstNonEmptyMessage(privateIP, device.PrivateIP)
			}
			providerTime := timeText(row.LastProviderTime)
			message = buildHoneypotAlertMessage(row.ProductType, row.Title, row.Severity, instanceName, instanceID, privateIP, "", providerTime, providerTime, departmentName(device), payload)
		case "custom_incident":
			item := aliyun.Incident{
				AccountType: row.ProductType,
				ProviderID:  row.ProviderAlertID,
				Title:       row.Title,
				Level:       row.Severity,
				CreateTime:  int64Value(payload["CreateTime"]),
				UpdateTime:  int64Value(payload["UpdateTime"]),
				Payload:     payload,
				Assets:      mapSlice(payload["Assets"]),
			}
			if item.CreateTime == 0 && row.LastProviderTime != nil {
				item.CreateTime = row.LastProviderTime.UnixMilli()
			}
			if device == nil {
				device = findIncidentDevice(item, payload)
			}
			message = buildIncidentAlertMessage(item, payload, device)
		case "core_file":
			item := aliyun.FileProtectEvent{
				AccountType:  row.ProductType,
				ProviderID:   row.ProviderAlertID,
				Title:        row.Title,
				Level:        row.Severity,
				InstanceID:   findValue(payload, "InstanceId", "InstanceID"),
				InstanceName: findValue(payload, "InstanceName", "HostName", "Hostname"),
				UUID:         findValue(payload, "Uuid", "UUID"),
				PrivateIP:    findValue(payload, "IntranetIp", "PrivateIP", "private_ip"),
				LastTime:     int64Value(firstNonEmptyAny(payload["LatestTime"], payload["LastTime"], payload["FirstTime"])),
			}
			message = buildCoreFileAlertMessage(item, payload)
		}
		if strings.TrimSpace(message) == "" {
			continue
		}
		var deviceID uint
		if device != nil {
			deviceID = device.ID
		}
		maskedMessage = s.maskMessage(message)
		if err := alertdao.UpdateSnapshot(row.ID, deviceID, maskedPayload, maskedMessage, alerttools.ContentHash(maskedMessage)); err != nil {
			return err
		}
		refreshed++
	}
	if refreshed > 0 {
		logger.Infof("sparse alert messages refreshed: count=%d", refreshed)
	}
	return nil
}

func needsMessageRefresh(row *alertmodel.SecurityAlert) bool {
	if row == nil {
		return false
	}
	rawMessage := row.LastMessage
	message := strings.TrimSpace(stripMarkdownBold(rawMessage))
	if strings.Contains(rawMessage, "\r") || containsMarkdownBoldLine(rawMessage) {
		return true
	}
	if row.AlertType == "suspicious_event" && isRootLoginAlert(row.Title) {
		return !strings.Contains(message, "等级标签："+coreSeverityText(row.Severity)) ||
			!strings.Contains(message, "\n【登录详情】\n")
	}
	if !isMissingMessageValue(row.Severity) {
		expectedSeverity := "告警等级：" + coreSeverityText(row.Severity)
		if !strings.Contains(message, expectedSeverity) {
			return true
		}
	}
	if strings.Contains(message, "告警详情：{") {
		return true
	}
	if row.AlertType == "suspicious_event" && containsMissingMessageField(message) {
		return true
	}
	if row.AlertType == "core_file" {
		for _, line := range strings.Split(message, "\n") {
			if (strings.Contains(line, "告警id：") || strings.Contains(line, "告警首次时间：") || strings.Contains(line, "告警末次时间：")) && strings.Contains(strings.ToLower(line), "e+") {
				return true
			}
		}
	}
	if row.AlertType == "honeypot" {
		return !strings.Contains(message, "🏷 内容：")
	}
	if row.AlertType != "custom_incident" {
		return false
	}
	marker := "🏷 内容："
	index := strings.Index(message, marker)
	if index < 0 {
		return true
	}
	body := strings.TrimSpace(message[index+len(marker):])
	if body == "" {
		return true
	}
	for _, line := range strings.Split(body, "\n") {
		if _, value, ok := strings.Cut(line, "--资产："); ok && net.ParseIP(strings.TrimSpace(value)) != nil {
			return true
		}
	}
	// Asset name and IP are useful for locating a host, but they do not explain
	// why a Cloud SIEM incident fired. Rebuild these historical snapshots so
	// newly supported detail fields (or an explicit provider-missing marker)
	// are included exactly once.
	for _, label := range []string{
		"--命令：", "--后续命令：", "--描述：", "--进程路径：", "--父进程路径：",
		"--检测原因：", "--风险原因：", "--事件明细：",
	} {
		if strings.Contains(body, label) {
			return false
		}
	}
	return true
}

func containsMarkdownBoldLine(message string) bool {
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if len(line) > 4 && strings.HasPrefix(line, "**") && strings.HasSuffix(line, "**") {
			return true
		}
	}
	return false
}

func containsMissingMessageField(message string) bool {
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		separator := strings.Index(line, "：")
		if separator < 0 {
			continue
		}
		if isMissingMessageValue(line[separator+len("："):]) {
			return true
		}
	}
	return false
}

func (s *Service) collectSuspEvents() error {
	start, end, err := s.alertWindow("suspicious_event")
	if err != nil {
		return err
	}
	events, err := s.clients.PullSuspEvents(alertPageSize, alertMaxPages, start, end)
	if err != nil {
		return err
	}
	createdOrChanged := 0
	autoWhitelisted := 0
	for _, item := range events {
		rawPayload := item.Payload
		// 安骑士已经自动加白的精准防御告警无需再次进入人工研判、
		// 入库或发送机器人通知。仅过滤自动加白成功，避免误丢手工
		// 加白、加白失败以及其他已处理状态的事件。
		if isAutoWhitelistedSuspiciousEvent(rawPayload) {
			autoWhitelisted++
			continue
		}
		payload := s.mask(rawPayload)
		// 保留 xops-warning 原有行为：普通登录类告警只接收 SSH；没有该
		// 明细字段的 Cloud SIEM 网络安全告警不受这个过滤条件影响。
		if !isSSHOrNonLoginEvent(payload) {
			continue
		}
		providerID := item.ProviderID
		if providerID == "" {
			providerID = item.UniqueInfo
		}
		fingerprintParts := []any{"suspicious_event", item.AccountType, providerID, item.UUID, item.InstanceID, item.Title}
		if providerID == "" {
			fingerprintParts = append(fingerprintParts, payload)
		}
		fingerprint := makeFingerprint(fingerprintParts...)
		lastProviderTime := parseProviderTime(item.LastTime, item.LastTimestamp)
		firstProviderTime := alerttools.FirstProviderTime("suspicious_event", rawPayload, lastProviderTime)
		device, _ := devicedao.Find(item.AccountType, item.UUID, item.InstanceID, item.InstanceName, item.PrivateIP)
		var deviceID uint
		if device != nil {
			deviceID = device.ID
		}
		message := s.maskMessage(buildSuspiciousAlertMessage(item, alertPayloadMap(payload), device))
		now := time.Now()
		row := &alertmodel.SecurityAlert{Source: "aliyun", ProductType: item.AccountType, AlertType: "suspicious_event", ProviderAlertID: providerID, Fingerprint: fingerprint, Title: item.Title, Severity: normalizeSuspiciousSeverity(item.Level), Status: "firing", DeviceID: deviceID, FirstProviderTime: firstProviderTime, LastSeenAt: now, LastProviderTime: lastProviderTime, OccurrenceCount: 1, LastPayload: marshal(payload), LastMessage: message, ContentHash: alerttools.ContentHash(message)}
		result, err := alertdao.Upsert(row, alerttools.EventKey(row.AlertType, row.Fingerprint, row.ProviderAlertID, row.LastProviderTime, rawPayload))
		if err != nil {
			return err
		}
		if result.Changed() {
			createdOrChanged++
			s.notifyCollected(row, result)
		}
	}
	logger.Infof("aliyun alerts collected: count=%d auto_whitelisted=%d new_or_changed=%d", len(events), autoWhitelisted, createdOrChanged)
	return s.commitCursor("suspicious_event", end, len(events))
}

// isAutoWhitelistedSuspiciousEvent identifies the provider's explicit
// "精准防御加白名单自动加白" success code.  Do not use EventStatus alone: it
// represents many states (ignored, manually confirmed and so on), none of
// which should be silently discarded.
func isAutoWhitelistedSuspiciousEvent(payload map[string]any) bool {
	return strings.EqualFold(
		strings.TrimSpace(findValue(payload, "OperateErrorCode")),
		"advance_mark_mis_info.System.Success",
	)
}

func (s *Service) collectAll() error {
	if err := s.collectSuspEvents(); err != nil {
		return err
	}
	if err := s.collectHoneypots(); err != nil {
		return err
	}
	if err := s.collectFileProtect(); err != nil {
		return err
	}
	return s.collectIncidents()
}

func (s *Service) collectHoneypots() error {
	events, err := s.clients.PullHoneypotEvents(alertPageSize, alertMaxPages)
	if err != nil {
		return err
	}
	changed := 0
	for _, item := range events {
		rawPayload := expandMergeFieldList(item.Payload)
		payload := s.mask(rawPayload)
		providerID := item.ProviderID
		fingerprintParts := []any{"honeypot", item.AccountType, providerID, item.Title, item.EventType}
		if providerID == "" {
			fingerprintParts = append(fingerprintParts, payload)
		}
		fingerprint := makeFingerprint(fingerprintParts...)
		last := parseProviderTime("", item.LastTime)
		device, _ := devicedao.Find(item.AccountType,
			findValue(rawPayload, "uuid", "Uuid", "UUID", "aliyun_uuid", "AliyunUUID"),
			findValue(rawPayload, "instanceId", "InstanceId", "instance_id", "InstanceID"),
			findValue(rawPayload, "instanceName", "InstanceName", "instance_name"),
			findValue(rawPayload, "dest_ip", "DestIp", "intranetIp", "IntranetIp", "private_ip", "PrivateIP"))
		instanceName := findValue(payload, "instanceName", "InstanceName", "instance_name")
		instanceID := findValue(payload, "instanceId", "InstanceId", "instance_id", "InstanceID")
		privateIP := findValue(payload, "dest_ip", "DestIp", "intranetIp", "IntranetIp", "private_ip", "PrivateIP")
		if device != nil {
			instanceName = firstNonEmptyMessage(instanceName, device.InstanceName)
			instanceID = firstNonEmptyMessage(instanceID, device.InstanceID)
			privateIP = firstNonEmptyMessage(privateIP, device.PrivateIP)
		}
		message := s.maskMessage(buildHoneypotAlertMessage(
			item.AccountType,
			item.Title,
			normalizeHoneypotSeverity(item.Level),
			instanceName,
			instanceID,
			privateIP,
			findValue(payload, "internetIp", "InternetIp", "public_ip", "PublicIP"),
			timeText(last), timeText(last), departmentName(device), alertPayloadMap(payload),
		))
		row := s.newAlert("aliyun", item.AccountType, "honeypot", providerID, fingerprint, item.Title, normalizeHoneypotSeverity(item.Level), device, alerttools.FirstProviderTime("honeypot", rawPayload, last), last, payload, message)
		result, err := alertdao.Upsert(row, alerttools.EventKey(row.AlertType, row.Fingerprint, row.ProviderAlertID, row.LastProviderTime, rawPayload))
		if err != nil {
			return err
		}
		if result.Changed() {
			changed++
			s.notifyCollected(row, result)
		}
	}
	logger.Infof("aliyun honeypot alerts collected: count=%d new_or_changed=%d", len(events), changed)
	return nil
}

func (s *Service) collectFileProtect() error {
	start, end, err := s.alertWindow("core_file")
	if err != nil {
		return err
	}
	events, err := s.clients.PullFileProtectEvents(alertPageSize, alertMaxPages, start, end)
	if err != nil {
		return err
	}
	changed := 0
	for _, item := range events {
		rawPayload := item.Payload
		payload := s.mask(rawPayload)
		fingerprintParts := []any{"file_protect", item.AccountType, item.ProviderID, item.UUID, item.InstanceID, item.Title}
		if item.ProviderID == "" {
			fingerprintParts = append(fingerprintParts, payload)
		}
		fingerprint := makeFingerprint(fingerprintParts...)
		last := parseProviderTime("", item.LastTime)
		device, _ := devicedao.Find(item.AccountType, item.UUID, item.InstanceID, item.InstanceName, item.PrivateIP)
		message := s.maskMessage(buildCoreFileAlertMessage(item, alertPayloadMap(payload)))
		row := s.newAlert("aliyun", item.AccountType, "core_file", item.ProviderID, fingerprint, item.Title, normalizeCoreFileSeverity(item.Level), device, alerttools.FirstProviderTime("core_file", rawPayload, last), last, payload, message)
		result, err := alertdao.Upsert(row, alerttools.EventKey(row.AlertType, row.Fingerprint, row.ProviderAlertID, row.LastProviderTime, rawPayload))
		if err != nil {
			return err
		}
		if result.Changed() {
			changed++
			s.notifyCollected(row, result)
		}
	}
	logger.Infof("aliyun core-file alerts collected: count=%d new_or_changed=%d", len(events), changed)
	return s.commitCursor("core_file", end, len(events))
}

func (s *Service) collectIncidents() error {
	start, end, err := s.alertWindow("custom_incident")
	if err != nil {
		return err
	}
	events, err := s.clients.PullIncidents(alertPageSize, alertMaxPages, start, end)
	if err != nil {
		return err
	}
	changed := 0
	for _, item := range events {
		rawPayload := item.Payload
		payload := s.mask(rawPayload)
		lastMillis := item.UpdateTime
		if lastMillis == 0 {
			lastMillis = item.CreateTime
		}
		last := parseProviderTime("", lastMillis)
		device := findIncidentDevice(item, rawPayload)
		fingerprint := makeFingerprint("custom_incident", item.AccountType, item.ProviderID)
		message := s.maskMessage(buildIncidentAlertMessage(item, alertPayloadMap(payload), device))
		first := parseProviderTime("", item.CreateTime)
		row := s.newAlert("aliyun", item.AccountType, "custom_incident", item.ProviderID, fingerprint, item.Title, normalizeIncidentSeverity(item.Level), device, first, last, payload, message)
		result, err := alertdao.Upsert(row, alerttools.EventKey(row.AlertType, row.Fingerprint, row.ProviderAlertID, row.LastProviderTime, rawPayload))
		if err != nil {
			return err
		}
		if item.AssetLookupFailed && row.DeviceID == 0 {
			count, fallback, retryErr := s.cursor.RecordIncidentAssetLookupFailure(row.Fingerprint, incidentAssetRetryLimit)
			if retryErr != nil {
				// Do not silently drop a security alert when Redis is unavailable.
				logger.Warnf("record cloud siem asset lookup retry failed: alert_id=%d error=%v", row.ID, retryErr)
				s.notifyCollected(row, alertdao.UpsertResult{NewAlert: true})
			} else if fallback {
				logger.Warnf("cloud siem asset lookup retries exhausted: alert_id=%d attempts=%d", row.ID, count)
				s.notifyCollected(row, alertdao.UpsertResult{NewAlert: true})
			}
			continue
		}
		if row.DeviceID != 0 {
			pending, retryErr := s.cursor.ClearIncidentAssetLookupFailure(row.Fingerprint)
			if retryErr != nil {
				logger.Warnf("clear cloud siem asset lookup retry failed: alert_id=%d error=%v", row.ID, retryErr)
			}
			if pending {
				// Asset resolution recovered after a delayed retry. Force one message
				// to the device's department even if the incident snapshot itself did
				// not change during the retry window.
				s.notifyCollected(row, alertdao.UpsertResult{NewAlert: true})
				changed++
				continue
			}
		}
		if result.Changed() {
			changed++
			s.notifyCollected(row, result)
		}
	}
	logger.Infof("cloud siem incidents collected: count=%d new_or_changed=%d", len(events), changed)
	return s.commitCursor("custom_incident", end, len(events))
}

func (s *Service) notifyCollected(row *alertmodel.SecurityAlert, result alertdao.UpsertResult) {
	if row == nil || !result.Changed() {
		return
	}
	force := result.NewAlert || result.ContentChanged || row.AlertType != "suspicious_event"
	allowed, err := s.cursor.ClaimNotification(row.Fingerprint, repeatedNotificationInterval, force)
	if err != nil {
		// Notification delivery is security-sensitive. Redis throttling failure is
		// fail-open so an infrastructure issue cannot silently drop an alert.
		logger.Warnf("claim alert notification window failed: alert_id=%d error=%v", row.ID, err)
		allowed = true
	}
	if allowed {
		if err := s.notifyMessage(row, "", "", nil); err != nil {
			logger.Warnf("send collected alert notification failed: alert_id=%d error=%v", row.ID, err)
		}
	}
}

func (s *Service) notifyMessage(row *alertmodel.SecurityAlert, by, note string, confirmedAt *time.Time) error {
	if s == nil || s.notifier == nil || row == nil {
		if row != nil {
			logger.Warnf("alert notification skipped: alert_id=%d reason=notification service is unavailable", row.ID)
		}
		return fmt.Errorf("通知服务不可用")
	}
	if !s.notifier.Enabled() {
		logger.Warnf("alert notification skipped: alert_id=%d reason=robot notifications are disabled", row.ID)
		return fmt.Errorf("机器人通知未启用")
	}
	device := s.notificationDevice(row)
	message := s.alertGroupMessage(row, device, by, note, confirmedAt)
	robotID := 0
	dutyUser := ""
	departmentID := uint(0)
	if device != nil {
		departmentID = device.DepartmentID
	}
	if departmentID == 0 {
		var err error
		robotID, err = departmentdao.GetFallbackRobotID()
		if err != nil {
			logger.Errorf("resolve fallback alert group failed: alert_id=%d error=%v", row.ID, err)
			return fmt.Errorf("读取未分组默认告警群失败：%w", err)
		}
		if robotID == 0 {
			logger.Warnf("alert notification skipped: alert_id=%d reason=no associated device or fallback group is configured", row.ID)
			return fmt.Errorf("告警未关联设备或部门，请在“部门告警群配置”中设置未分组告警默认群")
		}
	} else {
		department, err := departmentdao.Get(departmentID)
		if err != nil {
			logger.Errorf("resolve department alert group failed: alert_id=%d device_id=%d department_id=%d error=%v", row.ID, device.ID, departmentID, err)
			return fmt.Errorf("读取部门告警群配置失败：%w", err)
		}
		if department == nil || !department.Enabled {
			logger.Warnf("alert notification skipped: alert_id=%d department_id=%d reason=department alert group is disabled or missing", row.ID, departmentID)
			return fmt.Errorf("所属部门的告警群未启用")
		}
		if department.RobotID == 0 {
			logger.Warnf("alert notification skipped: alert_id=%d department_id=%d reason=no alert group is configured", row.ID, departmentID)
			return fmt.Errorf("所属部门尚未配置告警群")
		}
		robotID = department.RobotID
		dutyUser = strings.TrimSpace(string(department.DutyUserID))
	}
	route := "department"
	if departmentID == 0 {
		route = "fallback"
	}
	deviceID := uint(0)
	if device != nil {
		deviceID = device.ID
	}
	logger.Infof("routing alert notification: alert_id=%d alert_type=%s device_id=%d department_id=%d route=%s im_id=%d", row.ID, row.AlertType, deviceID, departmentID, route, robotID)
	message = appendDutyMention(message, dutyUser)
	if err := s.notifier.Notify(context.Background(), robotID, message); err != nil {
		logger.Warnf("send alert notification failed: alert_id=%d department_id=%d error=%v", row.ID, departmentID, err)
		return fmt.Errorf("发送告警群消息失败：%w", err)
	}
	return nil
}

func (s *Service) notifyConfirmedMessage(row *alertmodel.SecurityAlert, by, note string, confirmedAt time.Time) (bool, error) {
	if s == nil || s.notifier == nil || row == nil {
		return false, fmt.Errorf("通知服务不可用")
	}
	if err := s.notifyMessage(row, by, note, &confirmedAt); err != nil {
		return false, err
	}
	return true, nil
}

// notificationDevice resolves Cloud SIEM assets again immediately before a
// message is sent. The provider may expose the host association only in Assets
// (rather than AlertDetails), and this final check prevents a transient parse
// miss from routing an otherwise associated host to the unassigned robot.
func (s *Service) notificationDevice(row *alertmodel.SecurityAlert) *devicemodel.Device {
	if row == nil {
		return nil
	}
	if row.DeviceID != 0 {
		if device, err := devicedao.Get(row.DeviceID); err == nil && device != nil {
			return device
		}
	}
	if row.AlertType != "custom_incident" {
		return nil
	}
	payload := make(map[string]any)
	if err := jsonutil.Unmarshal([]byte(row.LastPayload), &payload); err != nil {
		logger.Warnf("decode cloud siem payload before notification failed: alert_id=%d error=%v", row.ID, err)
		return nil
	}
	item := aliyun.Incident{AccountType: row.ProductType, ProviderID: row.ProviderAlertID, Assets: mapSlice(payload["Assets"])}
	device := findIncidentDevice(item, payload)
	if device == nil && s.clients != nil && row.ProviderAlertID != "" {
		assets, err := s.clients.IncidentAssets(row.ProductType, row.ProviderAlertID)
		if err != nil {
			logger.Warnf("resolve cloud siem notification device failed: alert_id=%d incident_uuid=%s error=%v", row.ID, row.ProviderAlertID, err)
		} else {
			for _, asset := range assets {
				if device = findDeviceByAsset(row.ProductType, asset); device != nil {
					break
				}
			}
		}
	}
	if device == nil {
		return nil
	}
	row.DeviceID = device.ID
	if row.ID != 0 {
		if err := alertdao.BindDevice(row.ID, device.ID); err != nil {
			logger.Warnf("persist cloud siem notification device failed: alert_id=%d device_id=%d error=%v", row.ID, device.ID, err)
		}
	}
	return device
}

func appendDutyMention(message, dutyUser string) string {
	dutyUser = strings.TrimSpace(dutyUser)
	if dutyUser == "" {
		return message
	}
	if !strings.HasPrefix(dutyUser, "@") {
		dutyUser = "@" + dutyUser
	}
	return strings.TrimRight(message, "\n") + "\n" + dutyUser
}

func requestUserID(authUser *user.AuthUser) int {
	if authUser == nil {
		return 0
	}
	return authUser.UserId
}

func findIncidentDevice(item aliyun.Incident, payload map[string]any) *devicemodel.Device {
	// Incident Assets are the provider's direct asset association. Prefer them
	// to fields in AlertDetails, which can also contain a container entity UUID.
	for _, asset := range item.Assets {
		if device := findDeviceByAsset(item.AccountType, asset); device != nil {
			return device
		}
	}
	if device := findDeviceByValues(item.AccountType,
		findIncidentValue(payload, "uuid", "Uuid", "UUID", "aliyun_uuid", "AliyunUUID"),
		findIncidentValue(payload, "instanceId", "InstanceId", "instance_id", "InstanceID"),
		findIncidentValue(payload, "instanceName", "InstanceName", "instance_name"),
		findIncidentValue(payload, "intranetIp", "IntranetIp", "private_ip", "PrivateIP", "host_ip", "HostIp", "dest_ip", "DestIp")); device != nil {
		return device
	}
	return nil
}

func findDeviceByAsset(accountType string, asset map[string]any) *devicemodel.Device {
	values := incidentAssetDeviceValues(asset)
	return findDeviceByValues(accountType, values.uuid, values.instanceID, values.instanceName, values.privateIP)
}

type incidentDeviceValues struct {
	uuid         string
	instanceID   string
	instanceName string
	privateIP    string
}

// findIncidentValue is deliberately scoped to Cloud SIEM incidents. The Cloud
// SIEM SDK preserves AlertDetails as []map[string]any before persistence,
// while generic findValue only walks []any. Looking inside each detail here
// makes first-time routing independent of later rebinds after JSON decoding.
func findIncidentValue(payload map[string]any, keys ...string) string {
	if value := findValue(payload, keys...); value != "" {
		return value
	}
	for _, detail := range mapSlice(payload["AlertDetails"]) {
		if value := findValue(detail, keys...); value != "" {
			return value
		}
	}
	return ""
}

func incidentAssetDeviceValues(asset map[string]any) incidentDeviceValues {
	values := incidentDeviceValues{}
	for _, info := range mapSlice(asset["AssetInfo"]) {
		key := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(fmt.Sprint(info["Key"])), "-", "_"), ".", "_"))
		value := strings.TrimSpace(fmt.Sprint(info["Values"]))
		if value == "" || value == "<nil>" {
			continue
		}
		switch {
		case key == "uuid" || key == "host_uuid" || key == "hostuuid" || key == "aliyun_uuid" || key == "aliyunuuid":
			if values.uuid == "" {
				values.uuid = value
			}
		case strings.Contains(key, "instance") && strings.Contains(key, "id"):
			if values.instanceID == "" {
				values.instanceID = value
			}
		case strings.Contains(key, "instance") && strings.Contains(key, "name"), strings.Contains(key, "host_name"), strings.Contains(key, "hostname"):
			if values.instanceName == "" {
				values.instanceName = value
			}
		case key == "host_ip" || strings.Contains(key, "intranet_ip") || strings.Contains(key, "private_ip") || strings.HasSuffix(key, "_ip") && strings.Contains(key, "inner"):
			if values.privateIP == "" {
				values.privateIP = value
			}
		}
	}

	// Some asset responses expose identifiers as top-level fields rather than
	// inside AssetInfo. AssetName is useful as a last-resort match for host
	// assets, while AssetId is deliberately not treated as an Alibaba UUID.
	if values.uuid == "" {
		values.uuid = findValue(asset, "AssetUuid", "HostUuid", "HostUUID", "Uuid", "UUID")
	}
	if values.instanceName == "" {
		values.instanceName = findValue(asset, "AssetName", "InstanceName", "HostName", "Hostname")
	}
	if values.privateIP == "" {
		values.privateIP = findValue(asset, "IntranetIp", "IntranetIP", "PrivateIp", "PrivateIP", "HostIp", "host_ip")
	}
	return values
}

func findDeviceByValues(accountType, uuid, instanceID, instanceName, privateIP string) *devicemodel.Device {
	device, _ := devicedao.Find(accountType, uuid, instanceID, instanceName, privateIP)
	return device
}

func mapSlice(value any) []map[string]any {
	switch items := value.(type) {
	case []map[string]any:
		return items
	case []any:
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if row, ok := item.(map[string]any); ok {
				result = append(result, row)
			}
		}
		return result
	default:
		return nil
	}
}

func (s *Service) alertWindow(productType string) (int64, int64, error) {
	now := time.Now()
	start, end, found, err := s.cursor.Window(productType, now)
	if err != nil {
		return 0, 0, err
	}
	if found {
		return start, end, nil
	}
	// Redis 游标首次建立时，优先复用已有告警的时间，避免部署升级后
	// 再次扫描完整的 24 小时窗口；成功拉取后游标将由 Redis 持久化。
	latest, err := alertdao.LatestProviderTime(productType)
	if err != nil {
		return 0, 0, err
	}
	if latest != nil {
		start = latest.Add(-2 * time.Minute).UnixMilli()
	}
	return start, end, nil
}

func (s *Service) commitCursor(productType string, end int64, count int) error {
	return s.cursor.Commit(productType, end, count)
}

func (s *Service) newAlert(source, productType, alertType, providerID, fingerprint, title, severity string, device *devicemodel.Device, first, last *time.Time, payload any, message string) *alertmodel.SecurityAlert {
	now := time.Now()
	row := &alertmodel.SecurityAlert{Source: source, ProductType: productType, AlertType: alertType, ProviderAlertID: providerID, Fingerprint: fingerprint, Title: title, Severity: severity, Status: "firing", FirstProviderTime: first, LastSeenAt: now, LastProviderTime: last, OccurrenceCount: 1, LastPayload: marshal(payload), LastMessage: message, ContentHash: alerttools.ContentHash(message)}
	if device != nil {
		row.DeviceID = device.ID
	}
	return row
}

type alertDeviceScope struct {
	deviceIDs           []uint
	includeUnassociated bool
}

type alertDeviceScopeRequest struct {
	UserID       int
	Host         string
	IP           string
	DepartmentID uint
	DeviceID     uint
}

// resolveAlertDeviceScope 独立查询部门和设备，再用设备 ID 限制告警表查询。
// 列表页的设备名称、IP 和部门也会在告警页数据取回后统一在内存中组装。
func resolveAlertDeviceScope(req alertDeviceScopeRequest) (alertDeviceScope, error) {
	departments, err := departmentdao.List()
	if err != nil {
		return alertDeviceScope{}, err
	}
	allowedDepartments := make([]uint, 0, len(departments))
	for _, department := range departments {
		memberIDs := make([]int, 0)
		if len(department.MemberIDs) == 0 || jsonutil.Unmarshal([]byte(department.MemberIDs), &memberIDs) != nil {
			continue
		}
		for _, memberID := range memberIDs {
			if memberID == req.UserID {
				allowedDepartments = append(allowedDepartments, department.ID)
				break
			}
		}
	}
	var departmentFilter *uint
	if req.DepartmentID != 0 {
		departmentFilter = &req.DepartmentID
	}
	deviceIDs, err := devicedao.ListAlertScopeIDs(req.Host, req.IP, departmentFilter, req.DeviceID, allowedDepartments)
	if err != nil {
		return alertDeviceScope{}, err
	}
	return alertDeviceScope{
		deviceIDs:           deviceIDs,
		includeUnassociated: req.Host == "" && req.IP == "" && req.DepartmentID == 0 && req.DeviceID == 0,
	}, nil
}

func ensureAlertVisible(row *alertmodel.SecurityAlert, userID int) error {
	if row == nil {
		return gorm.ErrRecordNotFound
	}
	if row.DeviceID == 0 {
		return nil
	}
	scope, err := resolveAlertDeviceScope(alertDeviceScopeRequest{UserID: userID, DeviceID: row.DeviceID})
	if err != nil {
		return err
	}
	for _, id := range scope.deviceIDs {
		if id == row.DeviceID {
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

func toAlertListItems(rows []alertmodel.SecurityAlert) ([]alertval.AlertListItem, error) {
	sanitizeAlertRows(rows)
	items := make([]alertval.AlertListItem, 0, len(rows))
	deviceIDs := make([]uint, 0, len(rows))
	for i := range rows {
		item := alertval.AlertListItem{}
		_ = copier.Copy(&item, &rows[i])
		item.FirstSeenAt = rows[i].CreatedAt
		items = append(items, item)
		if rows[i].DeviceID != 0 {
			deviceIDs = append(deviceIDs, rows[i].DeviceID)
		}
	}
	devices, err := devicedao.ListByIDs(deviceIDs)
	if err != nil {
		return nil, err
	}
	departments, err := departmentdao.List()
	if err != nil {
		return nil, err
	}
	departmentNames := make(map[uint]string, len(departments))
	for _, department := range departments {
		departmentNames[department.ID] = department.Name
	}
	deviceMap := make(map[uint]*devicemodel.Device, len(devices))
	for i := range devices {
		deviceMap[devices[i].ID] = &devices[i]
	}
	for i := range items {
		device := deviceMap[items[i].DeviceID]
		if device == nil {
			continue
		}
		items[i].DeviceName = device.InstanceName
		items[i].DevicePrivateIP = device.PrivateIP
		items[i].DepartmentID = device.DepartmentID
		items[i].DepartmentName = departmentNames[device.DepartmentID]
	}
	return items, nil
}

func toAlertDetailResp(row *alertmodel.SecurityAlert) *alertval.AlertDetailResp {
	if row == nil {
		return nil
	}
	sanitizeAlertRow(row)
	resp := &alertval.AlertDetailResp{}
	_ = copier.Copy(resp, row)
	resp.FirstSeenAt = row.CreatedAt
	return resp
}

func findValue(value any, keys ...string) string {
	var found string
	var walk func(any)
	walk = func(current any) {
		if found != "" {
			return
		}
		switch item := current.(type) {
		case map[string]any:
			for key, child := range item {
				for _, target := range keys {
					if strings.EqualFold(key, target) {
						if text := messageValue(child); !isMissingMessageValue(text) {
							found = text
							return
						}
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range item {
				walk(child)
			}
		}
	}
	walk(value)
	return found
}

func expandMergeFieldList(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	expanded := make(map[string]any, len(payload)+len(mapSlice(payload["MergeFieldList"])))
	for key, value := range payload {
		expanded[key] = value
	}
	for _, field := range mapSlice(payload["MergeFieldList"]) {
		key := strings.TrimSpace(fmt.Sprint(field["FieldKey"]))
		value := field["FieldValue"]
		if isMissingMessageValue(key) || isMissingMessageValue(messageValue(value)) {
			continue
		}
		if current, exists := expanded[key]; !exists || isMissingMessageValue(messageValue(current)) {
			expanded[key] = value
		}
	}
	return expanded
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	default:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
		return parsed
	}
}

func isSSHOrNonLoginEvent(value any) bool {
	foundLoginProtocol := false
	isSSH := false
	var walk func(any)
	walk = func(current any) {
		switch item := current.(type) {
		case map[string]any:
			name := ""
			value := ""
			for key, child := range item {
				if strings.EqualFold(key, "NameDisplay") {
					name = fmt.Sprint(child)
				}
				if strings.EqualFold(key, "ValueDisplay") {
					value = fmt.Sprint(child)
				}
			}
			if name == "登录协议" {
				foundLoginProtocol = true
				isSSH = strings.EqualFold(strings.TrimSpace(value), "SSH")
			}
			for _, child := range item {
				walk(child)
			}
		case []any:
			for _, child := range item {
				walk(child)
			}
		}
	}
	walk(value)
	return !foundLoginProtocol || isSSH
}

func timeText(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}

func (s *Service) maskMessage(value string) string {
	value = formatAlertNotificationText(notificationPlainText(value))
	masked := s.mask(map[string]any{"message": value})
	if fields, ok := masked.(map[string]any); ok {
		if text, ok := fields["message"].(string); ok {
			return text
		}
	}
	return "******"
}

func stripMarkdownBold(value string) string {
	lines := strings.Split(value, "\n")
	inBold := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && strings.Trim(trimmed, "*") == "" {
			lines[i] = trimmed
			continue
		}
		if strings.HasPrefix(trimmed, "**") {
			trimmed = strings.TrimPrefix(trimmed, "**")
			inBold = true
		}
		if inBold && strings.HasSuffix(trimmed, "**") {
			trimmed = strings.TrimSuffix(trimmed, "**")
			inBold = false
		}
		lines[i] = strings.TrimSpace(trimmed)
	}
	return strings.Join(lines, "\n")
}

func formatAlertNotificationText(value string) string {
	value = stripMarkdownBold(value)
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		if strings.Contains(line, "处置建议：") {
			lines[i] = concealRecommendationURLs(line)
		}
	}
	return addAlertMessageIcons(strings.Join(lines, "\n"))
}

// concealRecommendationURLs keeps the human-readable document title while
// removing provider URLs from remediation text. Source-file download links are
// intentionally handled elsewhere and remain visible.
func concealRecommendationURLs(value string) string {
	var b strings.Builder
	for index := 0; index < len(value); {
		openOffset := strings.IndexByte(value[index:], '[')
		if openOffset < 0 {
			b.WriteString(value[index:])
			break
		}
		open := index + openOffset
		b.WriteString(value[index:open])
		closeOffset := strings.IndexByte(value[open+1:], ']')
		if closeOffset < 0 {
			b.WriteString(value[open:])
			break
		}
		close := open + 1 + closeOffset
		label := value[open+1 : close]
		next := close + 1
		for next < len(value) && (value[next] == ' ' || value[next] == '\t') {
			next++
		}
		if next+1 < len(value) && value[next] == '\\' && value[next+1] == '(' {
			next++
		}
		if next < len(value) && value[next] == '(' {
			if end := closingMarkdownParen(value, next); end >= 0 {
				b.WriteString(label)
				index = end + 1
				continue
			}
		}
		b.WriteString(value[open : close+1])
		index = close + 1
	}
	return stripBareHTTPURLs(b.String())
}

func closingMarkdownParen(value string, start int) int {
	depth := 0
	for index := start; index < len(value); index++ {
		switch value[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func stripBareHTTPURLs(value string) string {
	var b strings.Builder
	for index := 0; index < len(value); {
		httpOffset := strings.Index(value[index:], "http://")
		httpsOffset := strings.Index(value[index:], "https://")
		offset := httpOffset
		if offset < 0 || (httpsOffset >= 0 && httpsOffset < offset) {
			offset = httpsOffset
		}
		if offset < 0 {
			b.WriteString(value[index:])
			break
		}
		start := index + offset
		b.WriteString(value[index:start])
		end := start
		for end < len(value) && !strings.ContainsRune(" \t\r\n)]\"'", rune(value[end])) {
			end++
		}
		index = end
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(b.String(), "[]", ""), "\\()", ""))
}

func (s *Service) mask(value any) any {
	// Decode provider HTML before password matching, so text split across tags
	// cannot leave a secret fragment in either the stored payload or message.
	return s.masker.Apply(sanitizeHTMLFields(value))
}

func alertPayloadMap(value any) map[string]any {
	if payload, ok := value.(map[string]any); ok {
		cleaned, _ := sanitizeHTMLFields(payload).(map[string]any)
		if cleaned != nil {
			return cleaned
		}
		return payload
	}
	return map[string]any{}
}

// buildSuspiciousAlertMessage 生成安骑士主机安全告警通知。
// 该格式与历史 warning 项目的通知格式保持一致，字段值来自已经脱敏的 payload。
func buildSuspiciousAlertMessage(item aliyun.SuspEvent, payload map[string]any, devices ...*devicemodel.Device) string {
	if isRootLoginAlert(item.Title) {
		var device *devicemodel.Device
		if len(devices) > 0 {
			device = devices[0]
		}
		return buildLoginAlertMessage(loginAlertInfo{
			title:      item.Title,
			severity:   coreSeverityText(normalizeSuspiciousSeverity(item.Level)),
			providerID: firstNonEmptyMessage(item.ProviderID, item.UniqueInfo),
			alertTime:  firstNonEmptyMessage(item.LastTime, item.OccurrenceTime),
			assetName:  item.InstanceName,
			privateIP:  item.PrivateIP,
		}, payload, nil, device)
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("**❌❌❌❌%s❌❌❌❌**", item.AccountType))
	writeMarkdownField(&b, "☁ 告警来源", accountEditionText(item.AccountType))
	writeMarkdownField(&b, "提示", firstNonEmptyMessage(detailValue(payload, "提示"), suspiciousPrompt(item.Title, payload)))
	writeMarkdownField(&b, "内网IP", firstNonEmptyMessage(item.PrivateIP, findValue(payload, "IntranetIp", "PrivateIP", "private_ip")))
	writeMarkdownField(&b, "实例名称", firstNonEmptyMessage(item.InstanceName, findValue(payload, "InstanceName", "instance_name", "HostName", "Hostname")))
	writeMarkdownField(&b, "攻击阶段", messageValue(payload["Stages"]))
	writeMarkdownField(&b, "告警名称", item.Title)
	writeMarkdownField(&b, "告警等级", coreSeverityText(normalizeSuspiciousSeverity(item.Level)))
	writeMarkdownField(&b, "首次发生时间", item.OccurrenceTime)
	writeMarkdownField(&b, "最新发生时间", item.LastTime)
	writeMarkdownField(&b, "登录时间", detailValue(payload, "登录时间"))
	writeMarkdownField(&b, "登录账号", detailValue(payload, "登录账号", "账号"))
	writeMarkdownField(&b, "登录协议", detailValue(payload, "登录协议", "协议"))
	writeMarkdownField(&b, "登录源IP", detailValue(payload, "登录源IP", "登录来源IP", "来源IP"))
	writeMarkdownField(&b, "登录地", detailValue(payload, "登录地", "登录地点", "归属地"))

	reason := detailValue(payload, "告警原因", "原因", "异常行为描述")
	if reason == "" {
		reason = findValue(payload, "Desc", "Description", "desc", "description")
	}
	writeMarkdownField(&b, "告警原因", reason)
	writeSuspiciousImportantDetails(&b, payload)

	if offlineTime := detailValue(payload, "离线时间", "客户端离线时间"); offlineTime != "" {
		b.WriteString("\n")
		writeMarkdownField(&b, "离线时间", offlineTime)
	}
	writeMarkdownField(&b, "客户端状态", detailValue(payload, "客户端状态", "状态"))

	if description := detailValue(payload, "描述", "事件描述"); description != "" {
		b.WriteString("\n")
		writeMarkdownField(&b, "描述", description)
	}
	writeMarkdownField(&b, "处置建议", detailValue(payload, "处置建议", "处理建议", "建议"))
	return b.String()
}

// writeSuspiciousImportantDetails keeps the host-security fields needed for
// investigation without copying every provider metadata field into the robot
// message. Aliases are deliberately merged under one stable display label.
func writeSuspiciousImportantDetails(b *strings.Builder, payload map[string]any) {
	fields := []struct {
		label string
		names []string
	}{
		{label: "用户名", names: []string{"用户名", "用户"}},
		{label: "工具信息", names: []string{"工具信息"}},
		{label: "数据来源", names: []string{"数据来源"}},
		{label: "命令行", names: []string{"命令行", "进程命令行"}},
		{label: "进程路径", names: []string{"进程路径"}},
		{label: "进程ID", names: []string{"进程ID"}},
		{label: "父进程命令行", names: []string{"父进程命令行"}},
		{label: "父进程文件路径", names: []string{"父进程文件路径", "父进程路径"}},
		{label: "父进程ID", names: []string{"父进程ID"}},
		{label: "进程链", names: []string{"进程链", "父进程关系"}},
		{label: "目标进程路径", names: []string{"目标进程路径"}},
		{label: "目标进程ID", names: []string{"目标进程ID"}},
		{label: "文件路径", names: []string{"文件路径", "恶意文件路径", "文件名"}},
		{label: "恶意文件MD5", names: []string{"恶意文件MD5", "文件MD5", "MD5"}},
		{label: "恶意文件SHA256", names: []string{"恶意文件SHA256", "文件SHA256", "SHA256"}},
		{label: "文件创建用户", names: []string{"文件创建用户"}},
		{label: "文件修改时间", names: []string{"文件修改时间"}},
		{label: "样本家族与特征", names: []string{"样本家族与特征"}},
		{label: "源文件下载", names: []string{"源文件下载"}},
		{label: "检测方式", names: []string{"检测方式"}},
		{label: "扫描来源方式", names: []string{"扫描来源方式"}},
		{label: "操作类型", names: []string{"操作类型"}},
		{label: "处置动作", names: []string{"处置动作"}},
		{label: "恶意行为", names: []string{"恶意行为"}},
		{label: "K8s集群名称", names: []string{"K8s集群名称"}},
		{label: "K8s命名空间", names: []string{"K8s命名空间"}},
		{label: "K8s Pod", names: []string{"K8s Pod"}},
		{label: "K8s节点名称", names: []string{"K8s节点名称"}},
		{label: "容器名", names: []string{"容器名"}},
		{label: "容器ID", names: []string{"容器ID"}},
		{label: "镜像名", names: []string{"镜像名"}},
		{label: "镜像ID", names: []string{"镜像ID"}},
		{label: "容器视角文件路径", names: []string{"容器视角文件路径"}},
		{label: "容器视角进程路径", names: []string{"容器视角进程路径"}},
	}
	for _, field := range fields {
		if field.label == "源文件下载" {
			writeMarkdownField(b, field.label, sourceDownloadValue(payload, field.names...))
			continue
		}
		writeMarkdownField(b, field.label, detailValue(payload, field.names...))
	}
}

// sourceDownloadValue uses the provider's original value when it is a URL.
// DescribeSuspEvents commonly supplies a display value such as "下载" alongside
// the actual temporary download URL in Value.
func sourceDownloadValue(payload map[string]any, names ...string) string {
	for _, detail := range mapSlice(payload["Details"]) {
		name := firstNonEmptyMessage(messageValue(detail["NameDisplay"]), messageValue(detail["Name"]))
		matched := false
		for _, target := range names {
			if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(target)) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		for _, value := range []any{detail["Value"], detail["ValueDisplay"]} {
			if href := safeHTTPURL(messageValue(value)); href != "" {
				return href
			}
		}
		return firstNonEmptyMessage(messageValue(detail["ValueDisplay"]), messageValue(detail["Value"]))
	}
	return ""
}

func buildHoneypotAlertMessage(accountType, title, severity, instanceName, instanceID, privateIP, publicIP, firstTime, lastTime, department string, payload map[string]any) string {
	message := buildAlertMessage(accountType, title, severity, instanceName, instanceID, privateIP, publicIP, firstTime, lastTime, department, payload)
	lines := honeypotContentLines(payload)
	if len(lines) == 0 {
		return message
	}
	var b strings.Builder
	b.WriteString(message)
	b.WriteString("\n**🏷 内容：**")
	for _, line := range lines {
		b.WriteString("\n**  --")
		b.WriteString(line)
		b.WriteString("**")
	}
	return b.String()
}

func honeypotContentLines(payload map[string]any) []string {
	labels := map[string]string{
		"src_ip":         "来源IP",
		"agent_name":     "探针",
		"dest_ip":        "目标IP",
		"dest_port_list": "目标端口",
	}
	lines := make([]string, 0)
	for _, field := range mapSlice(payload["MergeFieldList"]) {
		key := strings.TrimSpace(fmt.Sprint(field["FieldKey"]))
		value := messageValue(field["FieldValue"])
		if isMissingMessageValue(key) || isMissingMessageValue(value) {
			continue
		}
		label := labels[strings.ToLower(key)]
		if label == "" {
			label = key
		}
		lines = append(lines, label+"："+value)
	}
	return lines
}

// buildCoreFileAlertMessage 生成核心文件告警通知。
func buildCoreFileAlertMessage(item aliyun.FileProtectEvent, payload map[string]any) string {
	var b strings.Builder
	b.WriteString("**❌❌❌❌ 核心文件告警 ❌❌❌❌**")
	writeMarkdownField(&b, "☁ 告警来源", accountEditionText(item.AccountType))
	ruleType := firstNonEmptyMessage(
		findValue(payload, "RuleName", "RuleType", "AlertType"),
		"核心文件变更",
	)
	if !strings.HasSuffix(strings.ToLower(ruleType), "_corefile") {
		ruleType += "_coreFile"
	}
	writeMarkdownField(&b, "❌告警类型", ruleType)
	writeMarkdownField(&b, "❌告警等级", coreSeverityText(firstNonEmptyMessage(findValue(payload, "AlertLevel"), item.Level)))
	writeMarkdownField(&b, "❌告警ip", firstNonEmptyMessage(findValue(payload, "IntranetIp", "PrivateIP"), item.PrivateIP))
	writeMarkdownField(&b, "❌告警主机", firstNonEmptyMessage(findValue(payload, "InstanceName", "HostName", "Hostname"), item.InstanceName))
	writeMarkdownField(&b, "❌告警文件路径", findValue(payload, "FilePath", "FileName"))
	writeMarkdownField(&b, "❌告警进程路径", findValue(payload, "ProcPath", "ProcessPath"))
	writeMarkdownField(&b, "❌事件命令行", findValue(payload, "CmdLine", "Cmdline", "CommandLine"))
	writeMarkdownField(&b, "❌操作方式", findValue(payload, "Operation", "OperateType"))
	writeMarkdownField(&b, "❌操作id", findValue(payload, "OperationId", "OperateId"))
	writeMarkdownField(&b, "❌操作用户", findValue(payload, "User", "Operator", "OperateUser"))
	writeMarkdownField(&b, "❌告警id", findValue(payload, "Id", "EventId"))
	writeMarkdownField(&b, "❌事件状态", coreStatusText(findValue(payload, "Status", "EventStatus")))
	writeMarkdownField(&b, "❌告警首次时间", formatMessageTime(findValue(payload, "FirstTime")))
	writeMarkdownField(&b, "❌告警末次时间", formatMessageTime(findValue(payload, "LatestTime", "LastTime")))
	return b.String()
}

// buildIncidentAlertMessage 生成 Cloud SIEM 网络安全告警通知。
func buildIncidentAlertMessage(item aliyun.Incident, payload map[string]any, devices ...*devicemodel.Device) string {
	var b strings.Builder
	writeMarkdownField(&b, "📑 告警名称", item.Title)
	writeMarkdownField(&b, "🆔 告警编号", item.ProviderID)
	writeMarkdownField(&b, "🚨 告警等级", coreSeverityText(normalizeIncidentSeverity(item.Level)))
	writeMarkdownField(&b, "⏱ 告警时间", formatMessageTime(item.CreateTime))
	writeMarkdownField(&b, "☁ 告警来源", accountEditionText(item.AccountType))
	b.WriteString("\n**🏷 内容：**")
	lines := incidentContentLines(payload)
	if len(devices) > 0 && devices[0] != nil {
		device := devices[0]
		fallback := make([]string, 0, 2)
		if device.InstanceName != "" && !hasIncidentLine(lines, "资产：") {
			fallback = append(fallback, "资产："+messageValue(device.InstanceName))
		}
		if device.PrivateIP != "" && !hasIncidentLine(lines, "ip：") {
			fallback = append(fallback, "ip："+messageValue(device.PrivateIP))
		}
		lines = append(fallback, lines...)
	}
	for _, line := range lines {
		b.WriteString("\n**  --")
		b.WriteString(line)
		b.WriteString("**")
	}
	return b.String()
}

func hasIncidentLine(lines []string, prefix string) bool {
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return true
		}
	}
	return false
}

func writeMarkdownField(b *strings.Builder, name, value string) {
	value = messageValue(value)
	if isMissingMessageValue(value) {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "**%s：%s**", name, value)
}

func detailValue(payload map[string]any, names ...string) string {
	for _, detail := range mapSlice(payload["Details"]) {
		name := messageValue(detail["NameDisplay"])
		if name == "" {
			name = messageValue(detail["Name"])
		}
		for _, target := range names {
			if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(target)) {
				value := messageValue(firstNonEmptyAny(detail["ValueDisplay"], detail["Value"]))
				if !isMissingMessageValue(value) {
					return value
				}
			}
		}
	}
	return ""
}

func firstNonEmptyMessage(values ...string) string {
	for _, value := range values {
		if text := messageValue(value); !isMissingMessageValue(text) {
			return text
		}
	}
	return ""
}

func firstNonEmptyAny(values ...any) any {
	for _, value := range values {
		if text := messageValue(value); !isMissingMessageValue(text) {
			return value
		}
	}
	return nil
}

func messageValue(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case map[string]any, []any:
		return normalizeAlertText(marshal(typed))
	case float64:
		return normalizeAlertText(strconv.FormatFloat(typed, 'f', -1, 64))
	case float32:
		return normalizeAlertText(strconv.FormatFloat(float64(typed), 'f', -1, 32))
	default:
		return normalizeAlertText(fmt.Sprint(value))
	}
}

// isMissingMessageValue identifies provider placeholders that mean a detail
// was not supplied. Keeping this check in the shared message path prevents
// empty rows such as "父进程ID：-" from leaking into any alert template.
func isMissingMessageValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "-", "--", "<nil>", "nil", "null", "none", "n/a", "na", "[]", "{}":
		return true
	default:
		return false
	}
}

func suspiciousPrompt(title string, payload map[string]any) string {
	if strings.Contains(title, "离线") || strings.Contains(findValue(payload, "AlarmEventName", "Name"), "离线") {
		return "云安全中心客户端出现离线"
	}
	return ""
}

func coreSeverityText(value string) string {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "1":
		return "严重"
	case "2":
		return "可疑"
	case "3":
		return "低危"
	case "4":
		return "提示"
	case "serious":
		return "严重"
	case "high":
		return "高危"
	case "suspicious":
		return "可疑"
	case "medium":
		return "中危"
	case "low":
		return "低危"
	case "info":
		return "提示"
	case "unknown":
		return "未标识"
	default:
		return value
	}
}

func coreStatusText(value string) string {
	switch strings.TrimSpace(value) {
	case "0":
		return "未处理"
	case "1":
		return "处理中"
	case "2":
		return "已处理"
	case "3":
		return "已忽略"
	default:
		return value
	}
}

func accountEditionText(accountType string) string {
	switch strings.ToUpper(strings.TrimSpace(accountType)) {
	case "AQS":
		return "企业版"
	case "ULT":
		return "旗舰版"
	default:
		return ""
	}
}

func (s *Service) alertGroupMessage(row *alertmodel.SecurityAlert, device *devicemodel.Device, by, note string, confirmedAt *time.Time) string {
	if isLoginGroupAlert(row) {
		message, _ := s.masker.Apply(loginGroupMessage(row, device, by, note, confirmedAt)).(string)
		return message
	}
	if confirmedAt == nil {
		return row.LastMessage
	}
	return buildConfirmedAlertMessage(row, by, note, *confirmedAt)
}

func buildConfirmedAlertMessage(row *alertmodel.SecurityAlert, by, note string, confirmedAt time.Time) string {
	originalMessage := strings.TrimSpace(formatAlertNotificationText(row.LastMessage))
	originalMessage = ensureAlertSeverity(originalMessage, row.Severity)
	confirmedMessage := strings.ReplaceAll(originalMessage, "❌", "✅")
	if !strings.Contains(originalMessage, "❌") {
		confirmedMessage = "✅✅✅✅✅✅"
		if originalMessage != "" {
			confirmedMessage += "\n" + originalMessage
		}
	}
	var b strings.Builder
	b.WriteString(confirmedMessage)
	writePlainMessageField(&b, "👤 确认人", by)
	writePlainMessageField(&b, "⏱ 确认时间", confirmedAt.Format(time.DateTime))
	writePlainMessageField(&b, "📝 确认信息", note)
	return b.String()
}

func addAlertMessageIcons(message string) string {
	icons := []struct {
		field string
		icon  string
	}{
		{"提示", "💡"},
		{"内网IP", "🌐"},
		{"实例名称", "🖥"},
		{"攻击阶段", "🎯"},
		{"告警名称", "📑"},
		{"告警等级", "🚨"},
		{"首次发生时间", "⏱"},
		{"最新发生时间", "⏱"},
		{"登录时间", "⏱"},
		{"登录账号", "👤"},
		{"登录协议", "🔐"},
		{"登录源IP", "🌐"},
		{"登录地", "📍"},
		{"告警原因", "⚠️"},
		{"用户名", "👤"},
		{"工具信息", "🧰"},
		{"数据来源", "📡"},
		{"命令行", "💻"},
		{"进程路径", "⚙️"},
		{"进程ID", "🆔"},
		{"父进程命令行", "💻"},
		{"父进程文件路径", "⚙️"},
		{"父进程ID", "🆔"},
		{"进程链", "🔗"},
		{"目标进程路径", "⚙️"},
		{"目标进程ID", "🆔"},
		{"文件路径", "📁"},
		{"恶意文件MD5", "🔐"},
		{"恶意文件SHA256", "🔐"},
		{"文件创建用户", "👤"},
		{"文件修改时间", "⏱"},
		{"样本家族与特征", "🧬"},
		{"源文件下载", "🔗"},
		{"检测方式", "🔎"},
		{"扫描来源方式", "🔎"},
		{"操作类型", "🎬"},
		{"处置动作", "🛡"},
		{"恶意行为", "☠️"},
		{"K8s集群名称", "☸️"},
		{"K8s命名空间", "☸️"},
		{"K8s Pod", "☸️"},
		{"K8s节点名称", "☸️"},
		{"容器名", "📦"},
		{"容器ID", "📦"},
		{"镜像名", "📦"},
		{"镜像ID", "📦"},
		{"容器视角文件路径", "📁"},
		{"容器视角进程路径", "⚙️"},
		{"离线时间", "⏱"},
		{"客户端状态", "🖥"},
		{"描述", "📝"},
		{"处置建议", "🛠"},
	}
	lines := strings.Split(message, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, item := range icons {
			if strings.HasPrefix(trimmed, item.field+"：") {
				lines[i] = item.icon + " " + trimmed
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

func writePlainMessageField(b *strings.Builder, name, value string) {
	value = messageValue(value)
	if isMissingMessageValue(value) {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "%s：%s", name, value)
}

func ensureAlertSeverity(message, severity string) string {
	severity = coreSeverityText(severity)
	if severity == "" || strings.Contains(message, "告警等级：") {
		return message
	}
	severityLine := "🚨 告警等级：" + severity
	lines := strings.Split(message, "\n")
	for i, line := range lines {
		if strings.Contains(line, "告警名称：") {
			lines = append(lines[:i+1], append([]string{severityLine}, lines[i+1:]...)...)
			return strings.Join(lines, "\n")
		}
	}
	if message == "" {
		return severityLine
	}
	return message + "\n" + severityLine
}

func formatMessageTime(value any) string {
	text := messageValue(value)
	if text == "" || text == "<nil>" {
		return ""
	}
	if timestamp, err := strconv.ParseInt(text, 10, 64); err == nil {
		if timestamp > 100000000000 {
			return time.UnixMilli(timestamp).Format(time.DateTime)
		}
		if timestamp > 1000000000 {
			return time.Unix(timestamp, 0).Format(time.DateTime)
		}
	}
	if parsed := parseProviderTime(text, 0); parsed != nil {
		return parsed.Format(time.DateTime)
	}
	return messageValue(value)
}

func incidentContentLines(payload map[string]any) []string {
	lines := make([]string, 0)
	alertDetails := mapSlice(payload["AlertDetails"])
	for _, alert := range alertDetails {
		lines = append(lines, incidentAlertContentLines(alert)...)
	}
	detailLineCount := len(lines)
	for _, asset := range mapSlice(payload["Assets"]) {
		assetName := incidentAssetName(asset)
		privateIP := incidentHostIP(asset)
		command := findValue(asset, "Command", "Cmdline", "CmdLine", "CommandLine", "ProcessCommandLine")
		if assetName != "" {
			lines = append(lines, "资产："+messageValue(assetName))
		}
		if privateIP != "" {
			lines = append(lines, "ip："+messageValue(privateIP))
		}
		if command != "" {
			lines = append(lines, "命令："+messageValue(command))
		}
	}
	if detailLineCount == 0 {
		for _, key := range []string{"Content", "Description", "IncidentDescription", "Desc"} {
			if value := messageValue(payload[key]); value != "" {
				lines = append(lines, value)
				detailLineCount++
				break
			}
		}
	}
	if detailLineCount == 0 {
		if len(alertDetails) == 0 {
			lines = append(lines, "事件明细：阿里云 Cloud SIEM 未返回关联告警明细")
		} else {
			lines = append(lines, "事件明细：上游明细中未包含可展示的命令或描述")
		}
	}
	return uniqueIncidentLines(lines)
}

func incidentAlertContentLines(alert map[string]any) []string {
	// Cloud SIEM returns AlertDetail, ExtendContent and EntityList as JSON
	// strings. Decode them before extracting fields so the notification gets
	// the actual alert content instead of an empty aggregate event.
	content := make(map[string]any, len(alert))
	// EntityList can include the source IP as a generic entity. It must not
	// overwrite the affected host fields supplied by the concrete alert.
	for _, entity := range jsonObjectList(alert["EntityList"]) {
		mergeMap(content, entity)
	}
	mergeMap(content, alert)
	mergeMap(content, jsonObject(alert["AlertDetail"]))
	mergeMap(content, jsonObject(alert["ExtendContent"]))
	if standard := jsonObject(content["ctdr_standard_alert"]); len(standard) > 0 {
		mergeMap(content, standard)
		mergeMap(content, jsonObject(standard["extend_content"]))
	}

	lines := make([]string, 0, 12)
	assetName := incidentAssetName(content)
	privateIP := incidentHostIP(content)
	username := findValue(content, "username", "user_name", "UserName")
	command := findValue(content, "Command", "Cmdline", "CmdLine", "CommandLine", "ProcessCommandLine", "cmdline", "command_line", "dump_cmdline")
	if command == "" {
		command = commandChainText(content["cmd_chain"])
	}
	postCommand := findValue(content, "post_cmdline")
	processPath := findValue(content, "proc_path", "process_path", "ProcessPath")
	parentProcessPath := findValue(content, "parent_proc_path", "parent_process_path", "ParentProcessPath")
	workingDirectory := findValue(content, "cwd", "working_directory", "WorkingDirectory")
	detectionReason := findValue(content, "detection_reason", "DetectionReason")
	riskReason := findValue(content, "dump_risk_reason", "risk_reason", "RiskReason")
	dumpTime := findValue(content, "dump_time", "DumpTime")
	description := findValue(content, "AlertDesc", "AlertDescription", "alert_description", "Description", "description")
	if usefulIncidentValue(assetName) != "" {
		lines = append(lines, "资产："+messageValue(assetName))
	}
	if usefulIncidentValue(privateIP) != "" {
		lines = append(lines, "ip："+messageValue(privateIP))
	}
	if usefulIncidentValue(username) != "" {
		lines = append(lines, "用户："+messageValue(username))
	}
	if usefulIncidentValue(command) != "" {
		lines = append(lines, "命令："+messageValue(command))
	}
	if usefulIncidentValue(postCommand) != "" {
		lines = append(lines, "后续命令："+messageValue(postCommand))
	}
	if usefulIncidentValue(processPath) != "" {
		lines = append(lines, "进程路径："+messageValue(processPath))
	}
	if usefulIncidentValue(parentProcessPath) != "" {
		lines = append(lines, "父进程路径："+messageValue(parentProcessPath))
	}
	if usefulIncidentValue(workingDirectory) != "" {
		lines = append(lines, "工作目录："+messageValue(workingDirectory))
	}
	if usefulIncidentValue(detectionReason) != "" {
		lines = append(lines, "检测原因："+messageValue(detectionReason))
	}
	if usefulIncidentValue(riskReason) != "" {
		lines = append(lines, "风险原因："+messageValue(riskReason))
	}
	if usefulIncidentValue(dumpTime) != "" {
		lines = append(lines, "导出时间："+messageValue(dumpTime))
	}
	if usefulIncidentValue(description) != "" && usefulIncidentValue(command) == "" {
		lines = append(lines, "描述："+messageValue(description))
	}
	return lines
}

// incidentAssetName keeps IP-valued provider entity labels out of the asset
// name field, allowing buildIncidentAlertMessage to use the linked device name.
func incidentAssetName(asset map[string]any) string {
	for _, key := range []string{"InstanceName", "instance_name", "HostName", "host_name", "AssetName", "asset_name", "entity_name"} {
		name := incidentDisplayValue(asset, key)
		if name != "" && net.ParseIP(name) == nil {
			return name
		}
	}
	for _, info := range mapSlice(asset["AssetInfo"]) {
		key := strings.ToLower(messageValue(info["Key"]))
		if strings.Contains(key, "instance") && strings.Contains(key, "name") || strings.Contains(key, "host_name") || strings.Contains(key, "hostname") {
			name := usefulIncidentValue(messageValue(info["Values"]))
			if name != "" && net.ParseIP(name) == nil {
				return name
			}
		}
	}
	return ""
}

func incidentHostIP(content map[string]any) string {
	if ip := incidentDisplayValue(content, "IntranetIp", "PrivateIP", "private_ip", "host_ip", "dest_ip", "ip"); ip != "" {
		return ip
	}
	// Preserve IP-only asset labels as addresses when no host name is known.
	for _, key := range []string{"AssetName", "asset_name", "entity_name"} {
		if ip := incidentDisplayValue(content, key); net.ParseIP(ip) != nil {
			return ip
		}
	}
	return incidentDisplayValue(content, "src_ip")
}

// incidentDisplayValue respects alias priority and checks direct fields before
// nested fields. Sorted traversal makes repeated rendering of one payload stable.
func incidentDisplayValue(value any, aliases ...string) string {
	var lookup func(any, string) string
	lookup = func(value any, alias string) string {
		switch item := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(item))
			for key := range item {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				if strings.EqualFold(key, alias) {
					if text := usefulIncidentValue(messageValue(item[key])); text != "" {
						return text
					}
				}
			}
			for _, key := range keys {
				if text := lookup(item[key], alias); text != "" {
					return text
				}
			}
		case []any:
			for _, child := range item {
				if text := lookup(child, alias); text != "" {
					return text
				}
			}
		case []map[string]any:
			for _, child := range item {
				if text := lookup(child, alias); text != "" {
					return text
				}
			}
		}
		return ""
	}
	for _, alias := range aliases {
		if text := lookup(value, alias); text != "" {
			return text
		}
	}
	return ""
}

func usefulIncidentValue(value string) string {
	value = strings.TrimSpace(messageValue(value))
	if isMissingMessageValue(value) {
		return ""
	}
	return value
}

func uniqueIncidentLines(lines []string) []string {
	result := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		result = append(result, line)
	}
	return result
}

func commandChainText(value any) string {
	commands := make([]string, 0)
	for _, item := range mapSlice(value) {
		keys := make([]string, 0, len(item))
		for key := range item {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if command := messageValue(item[key]); command != "" {
				commands = append(commands, command)
			}
		}
	}
	return strings.Join(commands, " | ")
}

func jsonObject(value any) map[string]any {
	if object, ok := value.(map[string]any); ok {
		return object
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return nil
	}
	var object map[string]any
	if err := jsonutil.Unmarshal([]byte(text), &object); err != nil {
		return nil
	}
	return object
}

func jsonObjectList(value any) []map[string]any {
	if items, ok := value.([]any); ok {
		return mapSlice(items)
	}
	if items, ok := value.([]map[string]any); ok {
		return items
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return nil
	}
	var items []map[string]any
	if err := jsonutil.Unmarshal([]byte(text), &items); err != nil {
		return nil
	}
	return items
}

func mergeMap(dst, src map[string]any) {
	for key, value := range src {
		dst[key] = value
	}
}

func buildAlertMessage(accountType, title, severity, instanceName, instanceID, privateIP, publicIP, firstTime, lastTime, department string, payload any) string {
	return buildHostSecurityAlertMessage(accountType, title, severity, instanceName, privateIP, firstTime, lastTime, alertPayloadMap(payload))
}

func buildHostSecurityAlertMessage(accountType, title, severity, instanceName, privateIP, firstTime, lastTime string, payload map[string]any) string {
	item := aliyun.SuspEvent{
		AccountType:    accountType,
		Title:          title,
		Level:          severity,
		InstanceName:   instanceName,
		PrivateIP:      privateIP,
		OccurrenceTime: firstTime,
		LastTime:       lastTime,
	}
	return buildSuspiciousAlertMessage(item, payload)
}

func formatAlertDetails(payload any) string {
	cleaned, _ := sanitizeHTMLFields(payload).(map[string]any)
	if cleaned == nil {
		return normalizeAlertText(fmt.Sprint(payload))
	}
	lines := make([]string, 0)
	for _, detail := range mapSlice(cleaned["Details"]) {
		name := normalizeAlertText(fmt.Sprint(detail["NameDisplay"]))
		value := normalizeAlertText(fmt.Sprint(detail["ValueDisplay"]))
		if value == "" || value == "<nil>" {
			value = normalizeAlertText(fmt.Sprint(detail["Value"]))
		}
		if name == "" || name == "<nil>" {
			if value != "" && value != "<nil>" {
				lines = append(lines, value)
			}
			continue
		}
		if value != "" && value != "<nil>" {
			lines = append(lines, fmt.Sprintf("%s：%s", name, value))
		}
	}
	if len(lines) > 0 {
		return strings.Join(lines, "\n")
	}
	if desc := normalizeAlertText(findValue(cleaned, "Desc", "Description", "desc", "description")); desc != "" {
		return desc
	}
	return normalizeAlertText(marshal(cleaned))
}

func departmentName(device *devicemodel.Device) string {
	if device == nil || device.DepartmentID == 0 {
		return ""
	}
	dept, _ := departmentdao.Get(device.DepartmentID)
	if dept == nil {
		return ""
	}
	return dept.Name
}

func makeFingerprint(parts ...any) string {
	var b strings.Builder
	for _, part := range parts {
		if m, ok := part.(map[string]any); ok {
			b.WriteString(marshal(m))
		} else {
			b.WriteString(fmt.Sprint(part))
		}
		b.WriteByte('|')
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func marshal(value any) string { b, _ := jsonutil.Marshal(value); return string(b) }

func parseProviderTime(text string, timestamp int64) *time.Time {
	if timestamp > 0 {
		t := time.UnixMilli(timestamp)
		return &t
	}
	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.ParseInLocation(layout, text, time.Local); err == nil {
			return &t
		}
	}
	return nil
}

func normalizeSeverity(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "serious", "critical", "urgent", "紧急", "严重":
		return "serious"
	case "high", "高危":
		return "high"
	case "suspicious", "可疑":
		return "suspicious"
	case "medium", "middle", "中危":
		return "medium"
	case "low", "低危":
		return "low"
	case "remind", "reminder", "info", "informational", "notice", "提醒", "提示", "信息":
		return "info"
	case "":
		return "unknown"
	default:
		return "unknown"
	}
}

// normalizeSuspiciousSeverity follows DescribeSuspEvents.Level:
// serious (紧急), suspicious (可疑), remind (提醒).
func normalizeSuspiciousSeverity(value string) string {
	return normalizeSeverity(value)
}

// normalizeHoneypotSeverity follows ListHoneypotEvents.RiskLevel:
// 4 (高风险), 3 (中风险), 2 (低风险).
func normalizeHoneypotSeverity(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if normalized := map[string]string{"4": "high", "3": "medium", "2": "low"}[value]; normalized != "" {
		return normalized
	}
	return normalizeSeverity(value)
}

// normalizeCoreFileSeverity follows the legacy ListFileProtectClientEvent
// AlertLevel semantics used by the provider payload and its display text.
func normalizeCoreFileSeverity(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if normalized := map[string]string{"1": "serious", "2": "suspicious", "3": "low", "4": "info"}[value]; normalized != "" {
		return normalized
	}
	return normalizeSeverity(value)
}

// normalizeIncidentSeverity follows Cloud SIEM 2024-12-12 ThreatLevel:
// 5 (严重), 4 (高危), 3 (中危), 2 (低危), 1 (信息).
func normalizeIncidentSeverity(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if normalized := map[string]string{"5": "serious", "4": "high", "3": "medium", "2": "low", "1": "info"}[value]; normalized != "" {
		return normalized
	}
	return normalizeSeverity(value)
}
