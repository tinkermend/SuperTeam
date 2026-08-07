package project

import "testing"

func TestHumanReadableFailureSummaryChineseLeadAndKnownEnglishDetail(t *testing.T) {
	t.Parallel()
	got := humanReadableFailureSummary(FailureFamilyTransientRuntime, "runtime node is not connected")
	if got != "执行环境暂时不可用：Runtime 节点未连接" {
		t.Fatalf("got %q", got)
	}
	got = humanReadableFailureSummary(FailureFamilyTransientProvider, "")
	if got != "执行器启动或运行失败" {
		t.Fatalf("empty raw: %q", got)
	}
}

func TestTaskResultFailureSummaryDefaultChinese(t *testing.T) {
	t.Parallel()
	if got := taskResultFailureSummary(TaskResultContract{}); got != "任务结果报告失败" {
		t.Fatalf("got %q", got)
	}
	if got := taskResultCancellationSummary(TaskResultContract{}); got != "任务结果报告已取消" {
		t.Fatalf("cancel: %q", got)
	}
}
