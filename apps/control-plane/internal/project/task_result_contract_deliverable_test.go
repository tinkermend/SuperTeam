package project

import (
	"encoding/json"
	"testing"
)

// 韧性缺陷家族#4:deliverables.value 为非字符串 JSON 时必须宽容接收,
// 否则真实执行完成的任务因写回 400 永久卡 running。
func TestTaskResultDeliverableValueToleratesAnyJSONType(t *testing.T) {
	cases := map[string]string{
		`{"name":"报告","value":"纯文本"}`:                      "纯文本",
		`{"name":"指标","value":{"cpu":"12%","mem":"3.1G"}}`: `{"cpu":"12%","mem":"3.1G"}`,
		`{"name":"清单","value":[1,2,3]}`:                    `[1,2,3]`,
		`{"name":"计数","value":42}`:                         `42`,
		`{"name":"开关","value":true}`:                       `true`,
		`{"name":"空值","value":null}`:                       ``,
		`{"name":"缺省"}`:                                    ``,
	}
	for input, want := range cases {
		var deliverable TaskResultDeliverable
		if err := json.Unmarshal([]byte(input), &deliverable); err != nil {
			t.Fatalf("unmarshal %s: %v", input, err)
		}
		if deliverable.Value != want {
			t.Fatalf("input %s: value = %q, want %q", input, deliverable.Value, want)
		}
	}
}

func TestTaskResultDeliverableKeepsOtherFields(t *testing.T) {
	var deliverable TaskResultDeliverable
	input := `{"name":"性能报告","kind":"report","value":{"p95":"120ms"},"ref":"artifact:1","summary":"摘要"}`
	if err := json.Unmarshal([]byte(input), &deliverable); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if deliverable.Name != "性能报告" || deliverable.Kind != "report" || deliverable.Ref != "artifact:1" || deliverable.Summary != "摘要" {
		t.Fatalf("fields lost: %#v", deliverable)
	}
	if deliverable.Value != `{"p95":"120ms"}` {
		t.Fatalf("value = %q", deliverable.Value)
	}
}
