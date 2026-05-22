package push

import (
	"encoding/json"
	"fmt"
	"strings"

	"pusher/internal/log"
	"pusher/internal/registry"
)

type PushTask struct {
	Targets []string
	Message json.RawMessage
}

type PushQueue struct {
	taskQueue chan PushTask
	registry  *registry.Registry
	workerNum int
}

func New(registry *registry.Registry, capacity, workerNum int) *PushQueue {
	return &PushQueue{
		taskQueue: make(chan PushTask, capacity),
		registry:  registry,
		workerNum: workerNum,
	}
}

func (q *PushQueue) Start() {
	for i := 0; i < q.workerNum; i++ {
		go q.worker(i)
	}
	log.Infof("push queue started with %d workers", q.workerNum)
}

func (q *PushQueue) Stop() {
	close(q.taskQueue)
}

func (q *PushQueue) Push(task PushTask) error {
	select {
	case q.taskQueue <- task:
		return nil
	default:
		return fmt.Errorf("push queue full")
	}
}

func (q *PushQueue) worker(id int) {
	for task := range q.taskQueue {
		q.process(task)
	}
}

func (q *PushQueue) process(task PushTask) {
	seen := make(map[string]bool)

	for _, target := range task.Targets {
		conns := q.matchTarget(target)
		for _, conn := range conns {
			if seen[conn.UserKey] {
				continue
			}
			seen[conn.UserKey] = true

			if conn.IsClosed() {
				q.registry.Unregister(conn.UserKey)
				continue
			}

			q.send(conn, task.Message)
		}
	}
}

func (q *PushQueue) matchTarget(target string) []*registry.Connection {
	if target == "*" {
		return q.registry.GetAll()
	}

	parts := strings.Split(target, ":")
	if len(parts) != 3 {
		return nil
	}

	channel := parts[0]
	group := parts[1]
	uuid := parts[2]

	if group == "*" && uuid == "*" {
		return q.registry.GetByChannel(channel)
	}

	if uuid == "*" {
		return q.registry.GetByGroup(channel, group)
	}

	userKey := channel + ":" + group + ":" + uuid
	conn := q.registry.GetByUser(userKey)
	if conn == nil {
		return nil
	}
	return []*registry.Connection{conn}
}

func (q *PushQueue) send(conn *registry.Connection, message json.RawMessage) {
	data := fmt.Sprintf("event: message\ndata: {\"channel\":\"%s\",\"group\":\"%s\",\"uuid\":\"%s\",\"message\":%s}\n\n",
		conn.Channel, conn.Group, conn.UUID, string(message))

	conn.Conn.Write([]byte(data))
	if f, ok := conn.Conn.(interface{ Flush() }); ok {
		f.Flush()
	}
}
