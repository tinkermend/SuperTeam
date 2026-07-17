// Package session 承载入站多轮表单的会话态(选项目→选模式→填内容)。
// 刻意只放内存(带 TTL):进程重启丢失即重来,这是"connector 不持业务状态"
// 的边界——表单半成品不是业务事实。
package session

import (
	"sync"
	"time"
)

type Stage string

const (
	StageIdle          Stage = "idle"
	StagePickProject   Stage = "pick_project"
	StagePickMode      Stage = "pick_mode"
	StageAwaitContent  Stage = "await_content"
	StageConfirm       Stage = "confirm"
)

type FormState struct {
	Stage       Stage
	UserID      string
	ProjectID   string
	ProjectName string
	Mode        string
	Title       string
	Content     string
	UpdatedAt   time.Time
}

type Store struct {
	mu     sync.Mutex
	ttl    time.Duration
	states map[string]FormState // key = open_id
	now    func() time.Time
}

func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &Store{ttl: ttl, states: map[string]FormState{}, now: time.Now}
}

// SetClock 测试注入时钟。
func (s *Store) SetClock(now func() time.Time) { s.now = now }

func (s *Store) Get(openID string) (FormState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[openID]
	if !ok {
		return FormState{}, false
	}
	if s.now().Sub(state.UpdatedAt) > s.ttl {
		delete(s.states, openID)
		return FormState{}, false
	}
	return state, true
}

func (s *Store) Put(openID string, state FormState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state.UpdatedAt = s.now()
	s.states[openID] = state
}

func (s *Store) Clear(openID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.states, openID)
}
