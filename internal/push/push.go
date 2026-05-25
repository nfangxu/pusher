package push

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"pusher/internal/log"
	"pusher/internal/registry"
)

type PushTask struct {
	Targets []string
	Message json.RawMessage
}

type PushQueue struct {
	taskQueue     chan PushTask
	registry      *registry.Registry
	workerNum     int
	fanOutWorkers int
}

func New(registry *registry.Registry, capacity, workerNum, fanOutWorkers int) *PushQueue {
	return &PushQueue{
		taskQueue:     make(chan PushTask, capacity),
		registry:      registry,
		workerNum:     workerNum,
		fanOutWorkers: fanOutWorkers,
	}
}

func (q *PushQueue) Start() {
	for i := 0; i < q.workerNum; i++ {
		go q.worker(i)
	}
	log.Infof("push queue started with %d workers, fan-out workers %d", q.workerNum, q.fanOutWorkers)
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
	var allConns []*registry.Connection

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

			allConns = append(allConns, conn)
		}
	}

	if len(allConns) == 0 {
		return
	}

	if len(allConns) <= 100 {
		for _, conn := range allConns {
			q.send(conn, task.Message)
		}
	} else {
		q.fanOut(allConns, task.Message)
	}
}

func (q *PushQueue) fanOut(conns []*registry.Connection, message json.RawMessage) {
	var msgBuf bytes.Buffer
	if err := json.Compact(&msgBuf, message); err != nil {
		log.Errorw("消息压缩失败", "error", err.Error())
		return
	}
	compactMsg := msgBuf.Bytes()

	workers := q.fanOutWorkers
	if workers > len(conns) {
		workers = len(conns)
	}

	chunkSize := (len(conns) + workers - 1) / workers

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(conns) {
			end = len(conns)
		}
		if start >= end {
			break
		}

		wg.Add(1)
		go func(chunk []*registry.Connection) {
			defer wg.Done()
			q.sendBatch(chunk, compactMsg)
		}(conns[start:end])
	}
	wg.Wait()
}

func (q *PushQueue) sendBatch(conns []*registry.Connection, compactMsg []byte) {
	var buf bytes.Buffer
	buf.Grow(len(compactMsg) + 200)

	for _, conn := range conns {
		if conn.IsClosed() {
			q.registry.Unregister(conn.UserKey)
			continue
		}

		buf.Reset()
		buf.WriteString("event: message\ndata: {\"channel\":\"")
		buf.WriteString(conn.Channel)
		buf.WriteString("\",\"group\":\"")
		buf.WriteString(conn.Group)
		buf.WriteString("\",\"uuid\":\"")
		buf.WriteString(conn.UUID)
		buf.WriteString("\",\"message\":")
		buf.Write(compactMsg)
		buf.WriteString("}\n\n")

		q.writeToConn(conn, buf.Bytes())
	}
}

func (q *PushQueue) writeToConn(conn *registry.Connection, data []byte) {
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

	conn.Conn.Write(data)
	if f, ok := conn.Conn.(interface{ Flush() }); ok {
		f.Flush()
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
