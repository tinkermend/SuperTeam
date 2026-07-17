// Package dedup 提供 event_id 幂等去重:飞书事件投递语义为 at-least-once,
// 超时/重连都会重推,业务处理前必须先判重。容量有界(近似 LRU),单副本内存态。
package dedup

import (
	"container/list"
	"sync"
)

type Set struct {
	mu       sync.Mutex
	capacity int
	order    *list.List
	index    map[string]*list.Element
}

func New(capacity int) *Set {
	if capacity <= 0 {
		capacity = 4096
	}
	return &Set{
		capacity: capacity,
		order:    list.New(),
		index:    make(map[string]*list.Element, capacity),
	}
}

// Seen 记录并返回该 ID 是否已出现过:首次返回 false,重复返回 true。
func (s *Set) Seen(id string) bool {
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if element, ok := s.index[id]; ok {
		s.order.MoveToFront(element)
		return true
	}
	element := s.order.PushFront(id)
	s.index[id] = element
	if s.order.Len() > s.capacity {
		oldest := s.order.Back()
		if oldest != nil {
			s.order.Remove(oldest)
			delete(s.index, oldest.Value.(string))
		}
	}
	return false
}
