package project

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
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

// v2 spec §3:契约落库前 deliverables[].ref 由相对路径/文件名回填为
// artifact_ref_id;匹配不到与纯 value 项原样保留。
func TestResolveDeclaredDeliverableRefs(t *testing.T) {
	reportID := uuid.New()
	dataID := uuid.New()
	declared := map[string]uuid.UUID{
		"deliverables/report.html": reportID,
		"report.html":              reportID,
		"deliverables/data.csv":    dataID,
		"data.csv":                 dataID,
	}
	contract := TaskResultContract{Deliverables: []TaskResultDeliverable{
		{Name: "report", Ref: "deliverables/report.html"},
		{Name: "data", Ref: "data.csv", Summary: "既有摘要"},
		{Name: "score", Value: "98"},
		{Name: "external", Ref: "https://example.com/x"},
	}}

	resolved := resolveDeclaredDeliverableRefs(contract, declared)

	if resolved.Deliverables[0].Ref != reportID.String() {
		t.Fatalf("report ref = %q, want %q", resolved.Deliverables[0].Ref, reportID.String())
	}
	if resolved.Deliverables[0].Summary != "deliverables/report.html" {
		t.Fatalf("原始路径应挪入空 Summary, got %q", resolved.Deliverables[0].Summary)
	}
	if resolved.Deliverables[1].Ref != dataID.String() {
		t.Fatalf("文件名匹配应生效, got %q", resolved.Deliverables[1].Ref)
	}
	if resolved.Deliverables[1].Summary != "既有摘要" {
		t.Fatalf("非空 Summary 不得覆盖, got %q", resolved.Deliverables[1].Summary)
	}
	if resolved.Deliverables[2].Value != "98" || resolved.Deliverables[2].Ref != "" {
		t.Fatalf("值型交付物不得改动")
	}
	if resolved.Deliverables[3].Ref != "https://example.com/x" {
		t.Fatalf("未匹配 Ref 应原样保留, got %q", resolved.Deliverables[3].Ref)
	}
}

func TestResolveDeclaredDeliverableRefsToleratesMissingPrefix(t *testing.T) {
	id := uuid.New()
	declared := map[string]uuid.UUID{"deliverables/out.md": id, "out.md": id}
	contract := TaskResultContract{Deliverables: []TaskResultDeliverable{
		{Name: "out", Ref: "out.md"},
	}}
	resolved := resolveDeclaredDeliverableRefs(contract, declared)
	if resolved.Deliverables[0].Ref != id.String() {
		t.Fatalf("缺前缀写法应命中, got %q", resolved.Deliverables[0].Ref)
	}
}
