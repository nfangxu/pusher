package push

import (
	"bytes"
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

	switch len(parts) {
	case 2:
		// {channel}:*
		channel := parts[0]
		if parts[1] == "*" {
			return q.registry.GetByChannel(channel)
		}
		return nil
	case 3:
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
	default:
		return nil
	}
}

func (q *PushQueue) send(conn *registry.Connection, message json.RawMessage) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorw("推送 panic，连接可能已断开",
				"channel", conn.Channel,
				"group", conn.Group,
				"uuid", conn.UUID,
				"recover", fmt.Sprintf("%v", r),
			)
			conn.Close()
			q.registry.Unregister(conn.UserKey)
		}
	}()

	// 压缩 message，移除换行和多余空白，避免破坏 SSE 格式
	var buf bytes.Buffer
	if err := json.Compact(&buf, message); err != nil {
		log.Errorw("消息压缩失败", "error", err.Error())
		return
	}

	data := fmt.Sprintf("event: message\ndata: {\"channel\":\"%s\",\"group\":\"%s\",\"uuid\":\"%s\",\"message\":%s}\n\n",
		conn.Channel, conn.Group, conn.UUID, buf.String())

	conn.Conn.Write([]byte(data))
	if f, ok := conn.Conn.(interface{ Flush() }); ok {
		f.Flush()
	}
}
