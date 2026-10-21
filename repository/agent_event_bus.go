package repository

import (
	"context"
	"sync"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
)

// AgentRunSubscription 订阅单个运行的内存事件：Wake 表示数据库里有新的持久化事件，
// Events 推送不落库的临时事件（流式正文、工具预告）。
type AgentRunSubscription struct {
	Wake   <-chan struct{}
	Events <-chan model.AgentEvent
	Close  func()
}

type agentRunSubscriber struct {
	wake   chan struct{}
	events chan model.AgentEvent
}

var agentRunBus = struct {
	sync.Mutex
	subscribers map[string]map[*agentRunSubscriber]struct{}
}{subscribers: map[string]map[*agentRunSubscriber]struct{}{}}

func SubscribeAgentRun(runID string) AgentRunSubscription {
	subscriber := &agentRunSubscriber{wake: make(chan struct{}, 1), events: make(chan model.AgentEvent, 256)}
	agentRunBus.Lock()
	if agentRunBus.subscribers[runID] == nil {
		agentRunBus.subscribers[runID] = map[*agentRunSubscriber]struct{}{}
	}
	agentRunBus.subscribers[runID][subscriber] = struct{}{}
	agentRunBus.Unlock()
	var once sync.Once
	return AgentRunSubscription{Wake: subscriber.wake, Events: subscriber.events, Close: func() {
		once.Do(func() {
			agentRunBus.Lock()
			defer agentRunBus.Unlock()
			delete(agentRunBus.subscribers[runID], subscriber)
			if len(agentRunBus.subscribers[runID]) == 0 {
				delete(agentRunBus.subscribers, runID)
			}
		})
	}}
}

// NotifyAgentRun 唤醒订阅者去数据库补读事件；多次通知会合并。
func NotifyAgentRun(runID string) {
	agentRunBus.Lock()
	defer agentRunBus.Unlock()
	for subscriber := range agentRunBus.subscribers[runID] {
		select {
		case subscriber.wake <- struct{}{}:
		default:
		}
	}
}

// PublishAgentRunEvent 推送不落库的临时事件；订阅者缓冲满时丢弃，不阻塞模型流。
func PublishAgentRunEvent(runID string, event model.AgentEvent) {
	event.RunID, event.Sequence = runID, 0
	agentRunBus.Lock()
	defer agentRunBus.Unlock()
	for subscriber := range agentRunBus.subscribers[runID] {
		select {
		case subscriber.events <- event:
		default:
		}
	}
}

type agentEventNotifyKey struct{}

type agentEventNotifications struct {
	sync.Mutex
	runIDs map[string]struct{}
}

func (n *agentEventNotifications) add(runID string) {
	n.Lock()
	defer n.Unlock()
	if n.runIDs == nil {
		n.runIDs = map[string]struct{}{}
	}
	n.runIDs[runID] = struct{}{}
}

func (n *agentEventNotifications) flush() {
	n.Lock()
	runIDs := n.runIDs
	n.runIDs = nil
	n.Unlock()
	for runID := range runIDs {
		NotifyAgentRun(runID)
	}
}

func withAgentEventNotifications(db *gorm.DB) (*gorm.DB, *agentEventNotifications) {
	parent := context.Background()
	if db.Statement != nil && db.Statement.Context != nil {
		parent = db.Statement.Context
	}
	pending := &agentEventNotifications{}
	return db.WithContext(context.WithValue(parent, agentEventNotifyKey{}, pending)), pending
}

// notifyAgentEventAfterCommit 在事务提交后再唤醒订阅者，避免读到未提交的数据。
func notifyAgentEventAfterCommit(tx *gorm.DB, runID string) {
	if tx != nil && tx.Statement != nil && tx.Statement.Context != nil {
		if pending, ok := tx.Statement.Context.Value(agentEventNotifyKey{}).(*agentEventNotifications); ok {
			pending.add(runID)
			return
		}
	}
	NotifyAgentRun(runID)
}
