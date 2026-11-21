package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"xops-aegis-guard/server/apiserver/alert/val"
	"xops-aegis-guard/util/httper"

	"github.com/gin-gonic/gin"
)

func TestSingleConfirmationNotificationFields(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	const reason = "所属部门尚未配置告警群"
	httper.HandleResponse(ctx, nil, &val.AlertDetailResp{NotificationError: reason})

	var response struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Data["notification_sent"] != false || response.Data["notification_error"] != reason {
		t.Fatalf("unexpected notification fields: %s", recorder.Body.String())
	}
}
