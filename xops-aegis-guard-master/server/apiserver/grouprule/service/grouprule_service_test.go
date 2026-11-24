package service

import (
	"testing"

	devicemodel "xops-aegis-guard/server/apiserver/device/model"
	rulemodel "xops-aegis-guard/server/apiserver/grouprule/model"
	"xops-aegis-guard/server/apiserver/grouprule/tools"
)

func TestPlanBigDBScreenshot(t *testing.T) {
	conditions, err := tools.CompileConditions([]rulemodel.Condition{
		{Operator: tools.OperatorContains, Value: "-bigdb-，-bigdata，-bigdbvm-"},
		{Operator: tools.OperatorNotContains, Value: "-db-，-dbvm-"},
	})
	if err != nil {
		t.Fatal(err)
	}
	rule := rulemodel.GroupRule{Name: "bigdb", DepartmentID: 3, Enabled: true}
	rule.ID = 36
	rules := []compiledRule{{rule: rule, conditions: conditions}}
	for _, test := range []struct {
		name       string
		hostname   string
		department uint
		source     string
		enabled    bool
		want       int
	}{
		{"screenshot cdh04 unassigned", "zy-nph-zf-test-bigdbvm-cdh04", 0, "", true, 1},
		{"screenshot peer", "zy-nph-zf-test-bigdbvm-cdh02", 0, "", true, 1},
		{"excluded db", "zy-nph-zf-test-bigdata-db-cdh04", 0, "", true, 0},
		{"excluded dbvm", "zy-nph-zf-test-bigdata-dbvm-cdh04", 0, "", true, 0},
		{"manual assignment retained", "zy-nph-zf-test-bigdbvm-cdh04", 7, devicemodel.AssignSourceManual, true, 0},
		{"disabled rule", "zy-nph-zf-test-bigdbvm-cdh04", 0, "", false, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			rules[0].rule.Enabled = test.enabled
			device := devicemodel.Device{InstanceName: test.hostname, DepartmentID: test.department, AssignSource: test.source}
			device.ID = 1
			_, plan, stale := planDevices(rules, []devicemodel.Device{device})
			if len(plan) != test.want || len(stale) != 0 {
				t.Fatalf("plan=%+v stale=%+v", plan, stale)
			}
			if len(plan) > 0 && plan[0].rule.ID != 36 {
				t.Fatalf("matched rule %d, want 36", plan[0].rule.ID)
			}
		})
	}
}
