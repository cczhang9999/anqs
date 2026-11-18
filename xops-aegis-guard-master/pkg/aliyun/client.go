package aliyun

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"xops-aegis-guard/config"
	"xops-aegis-guard/logger"
	"xops-aegis-guard/util/json"

	cloudsiem "github.com/alibabacloud-go/cloud-siem-20241212/client"
	openapi "github.com/alibabacloud-go/darabonba-openapi/client"
	cloudopenapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	sas "github.com/alibabacloud-go/sas-20181203/client"
	util "github.com/alibabacloud-go/tea-utils/service"
	"github.com/alibabacloud-go/tea/dara"
)

type AccountClient struct {
	Account config.Aliyun
	Client  *sas.Client
	SIEM    *cloudsiem.Client
	SIEMAPI *cloudopenapi.Client
}

type incidentAssetCacheEntry struct {
	UpdateTime int64
	Assets     []map[string]any
}

type incidentAlertCacheEntry struct {
	UpdateTime int64
	Alerts     []map[string]any
}

type Clients struct {
	Accounts []AccountClient

	incidentAssetMu    sync.Mutex
	incidentAssetCache map[string]incidentAssetCacheEntry
	incidentAlertMu    sync.Mutex
	incidentAlertCache map[string]incidentAlertCacheEntry
}

func NewClients(conf []config.Aliyun) (*Clients, error) {
	out := &Clients{}
	for _, account := range conf {
		if !isSupportedAccountType(account.Type) {
			continue
		}
		if strings.TrimSpace(account.Type) == "" || strings.TrimSpace(account.AccessKey) == "" || strings.TrimSpace(account.AccessKeySecret) == "" {
			continue
		}
		cfg := &openapi.Config{}
		cfg.SetAccessKeyId(account.AccessKey).SetAccessKeySecret(account.AccessKeySecret)
		if account.Endpoint != "" {
			cfg.SetEndpoint(account.Endpoint)
		}
		if account.RegionID != "" {
			cfg.SetRegionId(account.RegionID)
		}
		client, err := sas.NewClient(cfg)
		if err != nil {
			return nil, fmt.Errorf("create aliyun client %s: %w", account.Name, err)
		}
		cloudCfg := &cloudopenapi.Config{}
		cloudCfg.SetAccessKeyId(account.AccessKey).SetAccessKeySecret(account.AccessKeySecret)
		cloudCfg.SetRegionId(account.RegionID)
		if account.CloudSIEMEndpoint != "" {
			cloudCfg.SetEndpoint(account.CloudSIEMEndpoint)
		}
		siemClient, err := cloudsiem.NewClient(cloudCfg)
		if err != nil {
			return nil, fmt.Errorf("create cloud siem client %s: %w", account.Name, err)
		}
		siemAPI, err := cloudopenapi.NewClient(cloudCfg)
		if err != nil {
			return nil, fmt.Errorf("create cloud siem api client %s: %w", account.Name, err)
		}
		out.Accounts = append(out.Accounts, AccountClient{Account: account, Client: client, SIEM: siemClient, SIEMAPI: siemAPI})
	}
	out.incidentAssetCache = make(map[string]incidentAssetCacheEntry)
	out.incidentAlertCache = make(map[string]incidentAlertCacheEntry)
	return out, nil
}

type HoneypotEvent struct {
	AccountType string
	ProviderID  string
	Title       string
	EventType   string
	Level       string
	LastTime    int64
	Payload     map[string]any
}

type FileProtectEvent struct {
	AccountType  string
	ProviderID   string
	Title        string
	Level        string
	InstanceID   string
	InstanceName string
	UUID         string
	PrivateIP    string
	LastTime     int64
	Payload      map[string]any
}

type Incident struct {
	AccountType       string
	ProviderID        string
	Title             string
	Level             string
	CreateTime        int64
	UpdateTime        int64
	Payload           map[string]any
	Assets            []map[string]any
	AssetLookupFailed bool
}

func (c *Clients) PullHoneypotEvents(pageSize, maxPages int) ([]HoneypotEvent, error) {
	var result []HoneypotEvent
	for _, account := range c.Accounts {
		if !isSupportedAccount(account) {
			continue
		}
		for page := 1; maxPages <= 0 || page <= maxPages; page++ {
			req := (&sas.ListHoneypotAlarmEventsRequest{}).SetCurrentPage(int32(page)).SetPageSize(int32(pageSize)).SetDealed("n")
			resp, err := account.Client.ListHoneypotAlarmEventsWithOptions(req, &util.RuntimeOptions{})
			if err != nil {
				return nil, fmt.Errorf("list honeypot events account=%s page=%d: %w", account.Account.Name, page, err)
			}
			if resp == nil || resp.Body == nil || len(resp.Body.HoneypotAlarmEvents) == 0 {
				break
			}
			for _, item := range resp.Body.HoneypotAlarmEvents {
				if item == nil {
					continue
				}
				providerID := firstNonEmpty(stringValue(item.AlarmUniqueInfo), int64String(item.AlarmEventId))
				result = append(result, HoneypotEvent{AccountType: account.Account.Type, ProviderID: providerID, Title: stringValue(item.AlarmEventName), EventType: stringValue(item.AlarmEventType), Level: stringValue(item.RiskLevel), LastTime: int64Value(item.LastTime), Payload: structToMap(item)})
			}
			if resp.Body.PageInfo == nil || int32Value(resp.Body.PageInfo.TotalCount) <= int32(page*pageSize) {
				break
			}
		}
	}
	return result, nil
}

func (c *Clients) PullFileProtectEvents(pageSize, maxPages int, start, end int64) ([]FileProtectEvent, error) {
	var result []FileProtectEvent
	for _, account := range c.Accounts {
		// xops-warning 原实现只从 AQS 账号读取该兼容接口。
		if !strings.EqualFold(account.Account.Type, "AQS") {
			continue
		}
		for page := 1; maxPages <= 0 || page <= maxPages; page++ {
			query := map[string]any{"CurrentPage": page, "PageSize": pageSize, "StartTime": start, "EndTime": end}
			request := &openapi.OpenApiRequest{Query: map[string]*string{}}
			// AQS documents this legacy operation as a GET with query parameters.
			for key, value := range query {
				request.Query[key] = stringPtr(fmt.Sprint(value))
			}
			params := (&openapi.Params{}).SetAction("ListFileProtectClientEvent").SetVersion("2018-12-03").SetProtocol("HTTPS").SetPathname("/").SetMethod("GET").SetAuthType("AK").SetStyle("RPC").SetReqBodyType("formData").SetBodyType("json")
			body, err := account.Client.DoRequest(params, request, &util.RuntimeOptions{})
			if err != nil {
				return nil, fmt.Errorf("list file protect events account=%s page=%d: %w", account.Account.Name, page, err)
			}
			events := nestedList(body, "body", "EventList")
			if len(events) == 0 {
				break
			}
			for _, item := range events {
				instanceName := stringFromAny(item["InstanceName"])
				procPath := stringFromAny(item["ProcPath"])
				if !strings.Contains(instanceName, "prod") || strings.Contains(procPath, "/usr/local/aegis/") {
					continue
				}
				providerID := firstNonEmpty(stringFromAny(item["Id"]), stringFromAny(item["EventId"]))
				result = append(result, FileProtectEvent{AccountType: account.Account.Type, ProviderID: providerID, Title: "核心文件变更", Level: stringFromAny(item["AlertLevel"]), InstanceID: stringFromAny(item["InstanceId"]), InstanceName: instanceName, UUID: stringFromAny(item["Uuid"]), PrivateIP: stringFromAny(item["IntranetIp"]), LastTime: numberFromAny(item["LatestTime"], item["LastTime"], item["FirstTime"]), Payload: item})
			}
			if len(events) < pageSize {
				break
			}
		}
	}
	return result, nil
}

func (c *Clients) PullIncidents(pageSize, maxPages int, start, end int64) ([]Incident, error) {
	var result []Incident
	for _, account := range c.Accounts {
		if !isSupportedAccount(account) {
			continue
		}
		if account.SIEM == nil {
			continue
		}
		for page := 1; maxPages <= 0 || page <= maxPages; page++ {
			req := (&cloudsiem.ListIncidentsRequest{}).SetRegionId(account.Account.RegionID).SetPageNumber(int32(page)).SetPageSize(int32(pageSize)).SetIncidentStatus(0).SetLang("zh").SetStartTime(start).SetEndTime(end)
			resp, err := account.SIEM.ListIncidentsWithOptions(req, &dara.RuntimeOptions{})
			if err != nil {
				return nil, fmt.Errorf("list cloud siem incidents account=%s page=%d: %w", account.Account.Name, page, err)
			}
			if resp == nil || resp.Body == nil || len(resp.Body.Incidents) == 0 {
				break
			}
			for _, item := range resp.Body.Incidents {
				if item == nil || strings.Contains(stringValue(item.IncidentName), "自适应") {
					continue
				}
				payload := structToMap(item)
				providerID := stringValue(item.IncidentUuid)
				updateTime := int64Value(item.UpdateTime)
				assets, assetErr := c.pullIncidentAssets(account, providerID, updateTime)
				assetLookupFailed := assetErr != nil
				if assetErr != nil {
					// 资产详情只是设备关联的增强信息，不能因为单个事件详情
					// 没有权限或暂时失败而阻断四类告警的主流程。
					logger.Warnf("pull cloud siem incident assets failed: account=%s incident_uuid=%s error=%v", account.Account.Name, providerID, assetErr)
				} else if len(assets) > 0 {
					payload["Assets"] = assets
				}
				alertDetails, alertErr := c.pullIncidentAlertDetails(account, providerID, updateTime, assets)
				if alertErr != nil {
					// 事件聚合数据仍然可用，明细接口失败时保留主流程，
					// 下次事件更新时间变化后会再次尝试拉取明细。
					logger.Warnf("pull cloud siem incident alert details failed: account=%s incident_uuid=%s error=%v", account.Account.Name, providerID, alertErr)
				} else if len(alertDetails) > 0 {
					payload["AlertDetails"] = alertDetails
				}
				result = append(result, Incident{AccountType: account.Account.Type, ProviderID: providerID, Title: stringValue(item.IncidentName), Level: stringValue(item.ThreatLevel), CreateTime: int64Value(item.CreateTime), UpdateTime: updateTime, Payload: payload, Assets: assets, AssetLookupFailed: assetLookupFailed})
			}
			if int32Value(resp.Body.TotalCount) <= int32(page*pageSize) {
				break
			}
		}
	}
	return result, nil
}

// pullIncidentAlertDetails fetches the concrete alerts associated with a Cloud
// SIEM incident. DescribeCloudSiemEventDetail only returns incident-level
// aggregation data, while DescribeAlertsWithEvent contains AlertDetail,
// ExtendContent and EntityList. Cache by the provider update time so an
// unchanged incident does not cause another detail request every minute.
func (c *Clients) pullIncidentAlertDetails(account AccountClient, incidentUUID string, updateTime int64, assets []map[string]any) ([]map[string]any, error) {
	if account.SIEMAPI == nil || strings.TrimSpace(incidentUUID) == "" {
		return nil, nil
	}
	cacheKey := account.Account.Name + ":" + incidentUUID
	c.incidentAlertMu.Lock()
	if cached, ok := c.incidentAlertCache[cacheKey]; ok && cached.UpdateTime == updateTime {
		alerts := cloneMaps(cached.Alerts)
		c.incidentAlertMu.Unlock()
		return alerts, nil
	}
	c.incidentAlertMu.Unlock()

	pageSize := 100
	var result []map[string]any
	for page := 1; ; page++ {
		request := &cloudopenapi.OpenApiRequest{Query: map[string]*string{
			"IncidentUuid": stringPtr(incidentUUID),
			"CurrentPage":  stringPtr(strconv.Itoa(page)),
			"PageSize":     stringPtr(strconv.Itoa(pageSize)),
			"RegionId":     stringPtr(account.Account.RegionID),
			"RoleType":     stringPtr("0"),
		}}
		params := (&cloudopenapi.Params{}).
			SetAction("DescribeAlertsWithEvent").
			SetVersion("2022-06-16").
			SetProtocol("HTTPS").
			SetPathname("/").
			SetMethod("GET").
			SetAuthType("AK").
			SetStyle("RPC").
			SetReqBodyType("formData").
			SetBodyType("json")
		body, err := account.SIEMAPI.DoRequest(params, request, &dara.RuntimeOptions{})
		if err != nil {
			return nil, fmt.Errorf("describe cloud siem alert details page=%d: %w", page, err)
		}
		items := nestedListAny(body, "Data", "ResponseData")
		if len(items) == 0 {
			items = nestedListAny(body, "body", "Data", "ResponseData")
		}
		if len(items) == 0 {
			break
		}
		result = append(result, items...)
		if len(items) < pageSize {
			break
		}
	}
	expandJSONStrings(result)
	if len(result) == 0 {
		// Some custom Cloud SIEM incidents do not return data from the legacy
		// DescribeAlertsWithEvent API. Their related Assets still contain the
		// underlying AlertUuid, which can be resolved through the 2024 GetAlert
		// API. Only use this as a fallback to avoid adding requests for incidents
		// whose legacy detail is already complete.
		for _, alertUUID := range incidentAssetAlertUUIDs(assets) {
			alert, err := c.pullAlertByUUID(account, alertUUID)
			if err != nil {
				logger.Warnf("pull cloud siem alert by uuid failed: account=%s incident_uuid=%s alert_uuid=%s error=%v", account.Account.Name, incidentUUID, alertUUID, err)
				continue
			}
			if len(alert) > 0 {
				result = append(result, alert)
			}
		}
	}

	c.incidentAlertMu.Lock()
	c.incidentAlertCache[cacheKey] = incidentAlertCacheEntry{UpdateTime: updateTime, Alerts: cloneMaps(result)}
	c.incidentAlertMu.Unlock()
	return result, nil
}

func (c *Clients) pullAlertByUUID(account AccountClient, alertUUID string) (map[string]any, error) {
	alertUUID = strings.TrimSpace(alertUUID)
	if account.SIEMAPI == nil || alertUUID == "" {
		return nil, nil
	}
	request := &cloudopenapi.OpenApiRequest{Body: map[string]*string{
		"RegionId":  stringPtr(account.Account.RegionID),
		"Lang":      stringPtr("zh"),
		"AlertUuid": stringPtr(alertUUID),
		"RoleType":  stringPtr("0"),
	}}
	params := (&cloudopenapi.Params{}).
		SetAction("GetAlert").
		SetVersion("2024-12-12").
		SetProtocol("HTTPS").
		SetPathname("/").
		SetMethod("POST").
		SetAuthType("AK").
		SetStyle("RPC").
		SetReqBodyType("formData").
		SetBodyType("json")
	body, err := account.SIEMAPI.DoRequest(params, request, &dara.RuntimeOptions{})
	if err != nil {
		return nil, fmt.Errorf("get cloud siem alert uuid=%s: %w", alertUUID, err)
	}
	alert := cloudSIEMAlertRecord(body)
	if len(alert) == 0 {
		return nil, nil
	}
	// Keep the identifier in the same shape as DescribeAlertsWithEvent so the
	// occurrence key remains stable regardless of which API supplied details.
	if strings.TrimSpace(stringFromAny(mapValueFold(alert, "AlertUuid"))) == "" {
		alert["AlertUuid"] = alertUUID
	}
	expandJSONStrings(alert)
	return alert, nil
}

func incidentAssetAlertUUIDs(assets []map[string]any) []string {
	seen := make(map[string]struct{}, len(assets))
	result := make([]string, 0, len(assets))
	for _, asset := range assets {
		alertUUID := strings.TrimSpace(stringFromAny(mapValueFold(asset, "AlertUuid")))
		if alertUUID == "" {
			continue
		}
		if _, ok := seen[alertUUID]; ok {
			continue
		}
		seen[alertUUID] = struct{}{}
		result = append(result, alertUUID)
	}
	return result
}

func cloudSIEMAlertRecord(body map[string]any) map[string]any {
	record := nestedValueAny(body, "Alert", "AlertRecord")
	if record == nil {
		record = nestedValueAny(body, "body", "Alert", "AlertRecord")
	}
	switch value := record.(type) {
	case map[string]any:
		return value
	case string:
		var result map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &result); err == nil {
			return result
		}
	}
	return nil
}

// expandJSONStrings turns the JSON-string fields returned by Cloud SIEM into
// nested values before the payload enters masking and persistence. This keeps
// sensitive values inside AlertDetail/ExtendContent covered by the normal
// recursive desensitization path.
func expandJSONStrings(value any) any {
	switch item := value.(type) {
	case map[string]any:
		for key, child := range item {
			item[key] = expandJSONStrings(child)
		}
		return item
	case []any:
		for index, child := range item {
			item[index] = expandJSONStrings(child)
		}
		return item
	case []map[string]any:
		for index, child := range item {
			item[index] = expandJSONStrings(child).(map[string]any)
		}
		return item
	case string:
		text := strings.TrimSpace(item)
		if text == "" || (text[0] != '{' && text[0] != '[') {
			return item
		}
		var decoded any
		if err := json.Unmarshal([]byte(text), &decoded); err != nil {
			return item
		}
		return expandJSONStrings(decoded)
	default:
		return value
	}
}

// IncidentAssets returns the related assets for an existing incident. It is
// used to backfill alerts that were stored before Cloud SIEM asset enrichment
// was added. The same in-process cache is shared with PullIncidents.
func (c *Clients) IncidentAssets(accountType, incidentUUID string) ([]map[string]any, error) {
	for _, account := range c.Accounts {
		if !strings.EqualFold(account.Account.Type, accountType) || account.SIEMAPI == nil {
			continue
		}
		return c.pullIncidentAssets(account, incidentUUID, 0)
	}
	return nil, nil
}

// pullIncidentAssets fetches the Cloud SIEM assets related to one incident.
// The incident list is queried every minute, so the asset response is cached
// by incident UUID and provider update time. When the incident changes, the
// cache entry is refreshed automatically.
func (c *Clients) pullIncidentAssets(account AccountClient, incidentUUID string, updateTime int64) ([]map[string]any, error) {
	if account.SIEMAPI == nil || strings.TrimSpace(incidentUUID) == "" {
		return nil, nil
	}
	cacheKey := account.Account.Name + ":" + incidentUUID
	c.incidentAssetMu.Lock()
	if cached, ok := c.incidentAssetCache[cacheKey]; ok && cached.UpdateTime == updateTime {
		assets := cloneMaps(cached.Assets)
		c.incidentAssetMu.Unlock()
		return assets, nil
	}
	c.incidentAssetMu.Unlock()

	pageSize := 100
	var result []map[string]any
	for page := 1; ; page++ {
		request := &cloudopenapi.OpenApiRequest{Query: map[string]*string{
			"IncidentUuid": stringPtr(incidentUUID),
			"CurrentPage":  stringPtr(strconv.Itoa(page)),
			"PageSize":     stringPtr(strconv.Itoa(pageSize)),
			"RegionId":     stringPtr(account.Account.RegionID),
			"RoleType":     stringPtr("0"),
		}}
		params := (&cloudopenapi.Params{}).
			SetAction("DescribeCloudSiemAssets").
			SetVersion("2022-06-16").
			SetProtocol("HTTPS").
			SetPathname("/").
			SetMethod("GET").
			SetAuthType("AK").
			SetStyle("RPC").
			SetReqBodyType("formData").
			SetBodyType("json")
		body, err := account.SIEMAPI.DoRequest(params, request, &dara.RuntimeOptions{})
		if err != nil {
			return nil, fmt.Errorf("describe cloud siem assets page=%d: %w", page, err)
		}
		items := nestedListAny(body, "Data", "ResponseData")
		if len(items) == 0 {
			items = nestedListAny(body, "body", "Data", "ResponseData")
		}
		if len(items) == 0 {
			break
		}
		result = append(result, items...)
		if len(items) < pageSize {
			break
		}
	}

	c.incidentAssetMu.Lock()
	c.incidentAssetCache[cacheKey] = incidentAssetCacheEntry{UpdateTime: updateTime, Assets: cloneMaps(result)}
	c.incidentAssetMu.Unlock()
	return result, nil
}

type SuspEvent struct {
	AccountType    string
	ProviderID     string
	UniqueInfo     string
	Title          string
	Description    string
	Level          string
	InstanceID     string
	InstanceName   string
	UUID           string
	PrivateIP      string
	PublicIP       string
	LastTime       string
	OccurrenceTime string
	LastTimestamp  int64
	Payload        map[string]any
}

func (c *Clients) PullSuspEvents(pageSize, maxPages int, start, end int64) ([]SuspEvent, error) {
	var result []SuspEvent
	for _, account := range c.Accounts {
		if !isSupportedAccount(account) {
			continue
		}
		for page := 1; maxPages <= 0 || page <= maxPages; page++ {
			// 仅拉取待处理告警。安骑士会继续返回已处理、加白和误报记录，
			// 但它们不应再次进入本系统的告警、入库或机器人通知流程。
			req := (&sas.DescribeSuspEventsRequest{}).SetCurrentPage(strconv.Itoa(page)).SetPageSize(strconv.Itoa(pageSize)).SetStatus("1").SetTimeStart(formatTime(start)).SetTimeEnd(formatTime(end)).SetLang("zh")
			resp, err := account.Client.DescribeSuspEventsWithOptions(req, &util.RuntimeOptions{})
			if err != nil {
				return nil, fmt.Errorf("describe suspicious events account=%s page=%d: %w", account.Account.Name, page, err)
			}
			if resp == nil || resp.Body == nil || len(resp.Body.SuspEvents) == 0 {
				break
			}
			for _, item := range resp.Body.SuspEvents {
				if item == nil {
					continue
				}
				payload := structToMap(item)
				result = append(result, SuspEvent{
					AccountType: account.Account.Type, ProviderID: firstNonEmpty(int64String(item.Id), stringValue(item.UniqueInfo, item.AlarmUniqueInfo)),
					UniqueInfo: stringValue(item.UniqueInfo, item.AlarmUniqueInfo), Title: stringValue(item.AlarmEventNameDisplay, item.Name, item.AlarmEventName),
					Description: stringValue(item.Desc), Level: stringValue(item.Level), InstanceID: stringValue(item.InstanceId),
					InstanceName: stringValue(item.InstanceName), UUID: stringValue(item.Uuid), PrivateIP: stringValue(item.IntranetIp),
					PublicIP: stringValue(item.InternetIp), LastTime: stringValue(item.LastTime), OccurrenceTime: stringValue(item.OccurrenceTime),
					LastTimestamp: int64Value(item.LastTimeStamp), Payload: payload,
				})
			}
			if int32Value(resp.Body.TotalCount) <= int32(page*pageSize) {
				break
			}
		}
	}
	return result, nil
}

type Device struct {
	AccountType                    string
	UUID, InstanceID, InstanceName string
	PrivateIP, OSName              string
}

func (c *Clients) PullDevices(pageSize, maxPages int) ([]Device, error) {
	var result []Device
	for _, account := range c.Accounts {
		if !isSupportedAccount(account) {
			continue
		}
		for page := 1; maxPages <= 0 || page <= maxPages; page++ {
			req := (&sas.DescribeCloudCenterInstancesRequest{}).SetCurrentPage(int32(page)).SetPageSize(int32(pageSize)).SetLang("zh")
			resp, err := account.Client.DescribeCloudCenterInstancesWithOptions(req, &util.RuntimeOptions{})
			if err != nil {
				return nil, fmt.Errorf("describe devices account=%s page=%d: %w", account.Account.Name, page, err)
			}
			if resp == nil || resp.Body == nil || len(resp.Body.Instances) == 0 {
				break
			}
			for _, item := range resp.Body.Instances {
				if item == nil || strings.TrimSpace(stringValue(item.Uuid)) == "" {
					continue
				}
				result = append(result, Device{AccountType: account.Account.Type, UUID: stringValue(item.Uuid), InstanceID: stringValue(item.InstanceId), InstanceName: stringValue(item.InstanceName), PrivateIP: stringValue(item.IntranetIp), OSName: stringValue(item.OsName, item.Os)})
			}
			if resp.Body.PageInfo == nil || int32Value(resp.Body.PageInfo.TotalCount) <= int32(page*pageSize) {
				break
			}
		}
	}
	return result, nil
}

func isSupportedAccount(account AccountClient) bool {
	return isSupportedAccountType(account.Account.Type)
}

// isSupportedAccountType covers both cloud security center editions that use
// the shared asset, suspicious-event, honeypot, and Cloud SIEM APIs.
func isSupportedAccountType(accountType string) bool {
	switch strings.ToUpper(strings.TrimSpace(accountType)) {
	case "AQS", "ULT":
		return true
	default:
		return false
	}
}

func structToMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func stringValue(values ...*string) string {
	for _, v := range values {
		if v != nil && strings.TrimSpace(*v) != "" {
			return *v
		}
	}
	return ""
}
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func int64String(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}
func int64Value(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
func int32Value(v *int32) int32 {
	if v == nil {
		return 0
	}
	return *v
}

func stringPtr(value string) *string { return &value }

func formatTime(value int64) string {
	return time.UnixMilli(value).Format(time.DateTime)
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}

func numberFromAny(values ...any) int64 {
	for _, value := range values {
		text := strings.TrimSpace(stringFromAny(value))
		if text == "" {
			continue
		}
		if n, err := strconv.ParseInt(text, 10, 64); err == nil {
			return n
		}
		if n, err := strconv.ParseFloat(text, 64); err == nil {
			return int64(n)
		}
	}
	return 0
}

func nestedList(root map[string]interface{}, path ...string) []map[string]any {
	return nestedListAny(root, path...)
}

func nestedListAny(root map[string]any, path ...string) []map[string]any {
	current := nestedValueAny(root, path...)
	items, ok := current.([]interface{})
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, m)
		}
	}
	return result
}

func nestedValueAny(root map[string]any, path ...string) any {
	var current any = root
	for _, key := range path {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = mapValueFold(m, key)
	}
	return current
}

func mapValueFold(value map[string]any, key string) any {
	for name, item := range value {
		if strings.EqualFold(name, key) {
			return item
		}
	}
	return nil
}

func cloneMaps(values []map[string]any) []map[string]any {
	if len(values) == 0 {
		return nil
	}
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		clone := make(map[string]any, len(value))
		for key, item := range value {
			clone[key] = item
		}
		result = append(result, clone)
	}
	return result
}
