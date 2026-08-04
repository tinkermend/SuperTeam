package project

import (
	"testing"

	"github.com/google/uuid"
)

// 只有链**尾**可以接续：从链中间再接一条会把线性链分叉，而「第 k / n 次」与
// 卷宗链条展示都建立在线性之上。
func TestEvaluateDemandContinuationOnlyAllowsChainTail(t *testing.T) {
	running := ProjectDemand{ID: uuid.New(), Status: ProjectDemandStatusExecuting}
	settled := ProjectDemand{ID: uuid.New(), Status: ProjectDemandStatusCompleted}
	liveProject := Project{Status: ProjectStatusRunning}

	tail := evaluateDemandContinuation(liveProject, settled, 0, true)
	if !tail.Available || tail.ReasonCode != DemandContinuationReasonOK {
		t.Fatalf("链尾应可接续，得到 %#v", tail)
	}

	middle := evaluateDemandContinuation(liveProject, settled, 0, false)
	if middle.Available {
		t.Fatal("链中间不得可接续（会分叉）")
	}
	if middle.ReasonCode != DemandContinuationReasonAlreadyContinued {
		t.Fatalf("原因码应为 already_continued，得到 %q", middle.ReasonCode)
	}
	if middle.ReasonMessage == "" {
		t.Fatal("不可接续必须给中文原因，否则界面只能显示一颗点不动的按钮")
	}

	inFlight := evaluateDemandContinuation(liveProject, running, 0, true)
	if inFlight.Available || inFlight.ReasonCode != DemandContinuationReasonNotSettled {
		t.Fatalf("未结束的单不得可接续，得到 %#v", inFlight)
	}

	// 归档项目：SubmitDemand 会拒，判据必须同步表达，否则界面说能接、点了报错。
	archived := evaluateDemandContinuation(Project{Status: ProjectStatusArchived}, settled, 0, true)
	if archived.Available || archived.ReasonCode != DemandContinuationReasonProjectArchived {
		t.Fatalf("归档项目不得可接续，得到 %#v", archived)
	}

	deep := evaluateDemandContinuation(liveProject, settled, DefaultDemandContinuationMaxDepth, true)
	if deep.Available || deep.ReasonCode != DemandContinuationReasonChainTooDee {
		t.Fatalf("超深链不得可接续，得到 %#v", deep)
	}
}

func TestIsChainTail(t *testing.T) {
	head := ProjectDemand{ID: uuid.New()}
	tail := ProjectDemand{ID: uuid.New()}
	chain := []ProjectDemand{head, tail}

	if !isChainTail(chain, tail.ID) {
		t.Fatal("末元素应判为链尾")
	}
	if isChainTail(chain, head.ID) {
		t.Fatal("链头在有后继时不是链尾")
	}
	// 链读取降级成空时保守放行：宁可多放一次接续，也不因读不出链把功能锁死。
	if !isChainTail(nil, head.ID) {
		t.Fatal("空链应保守判为可接续")
	}
}
