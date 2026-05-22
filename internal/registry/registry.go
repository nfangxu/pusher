package registry

import (
	"hash/fnv"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Connection struct {
	UserKey   string
	Channel   string
	Group     string
	UUID      string
	Conn      http.ResponseWriter
	Done      chan struct{}
	CreatedAt time.Time
	mu        sync.Mutex
}

func (c *Connection) Close() {
	select {
	case <-c.Done:
	default:
		close(c.Done)
	}
}

func (c *Connection) IsClosed() bool {
	select {
	case <-c.Done:
		return true
	default:
		return false
	}
}

type RegistryShard struct {
	byUser    map[string]*Connection
	byGroup   map[string][]*Connection
	byChannel map[string][]*Connection
	mu        sync.RWMutex
}

type Registry struct {
	shards   []*RegistryShard
	shardNum uint64
}

func New(shardNum int) *Registry {
	r := &Registry{
		shards:   make([]*RegistryShard, shardNum),
		shardNum: uint64(shardNum),
	}
	for i := 0; i < shardNum; i++ {
		r.shards[i] = &RegistryShard{
			byUser:    make(map[string]*Connection),
			byGroup:   make(map[string][]*Connection),
			byChannel: make(map[string][]*Connection),
		}
	}
	return r
}

func (r *Registry) getShard(channel string) *RegistryShard {
	h := fnv.New32a()
	h.Write([]byte(channel))
	return r.shards[h.Sum32()%uint32(r.shardNum)]
}

func (r *Registry) Register(conn *Connection) {
	shard := r.getShard(conn.Channel)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if old, exists := shard.byUser[conn.UserKey]; exists {
		old.Close()
		r.removeFromIndices(shard, old)
	}

	shard.byUser[conn.UserKey] = conn

	groupKey := conn.Channel + ":" + conn.Group
	shard.byGroup[groupKey] = append(shard.byGroup[groupKey], conn)

	shard.byChannel[conn.Channel] = append(shard.byChannel[conn.Channel], conn)
}

func (r *Registry) Unregister(userKey string) {
	parts := strings.Split(userKey, ":")
	if len(parts) != 3 {
		return
	}
	channel := parts[0]

	shard := r.getShard(channel)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if conn, exists := shard.byUser[userKey]; exists {
		conn.Close()
		r.removeFromIndices(shard, conn)
		delete(shard.byUser, userKey)
	}
}

func (r *Registry) removeFromIndices(shard *RegistryShard, conn *Connection) {
	groupKey := conn.Channel + ":" + conn.Group
	if conns, ok := shard.byGroup[groupKey]; ok {
		for i, c := range conns {
			if c.UserKey == conn.UserKey {
				shard.byGroup[groupKey] = append(conns[:i], conns[i+1:]...)
				break
			}
		}
		if len(shard.byGroup[groupKey]) == 0 {
			delete(shard.byGroup, groupKey)
		}
	}

	if conns, ok := shard.byChannel[conn.Channel]; ok {
		for i, c := range conns {
			if c.UserKey == conn.UserKey {
				shard.byChannel[conn.Channel] = append(conns[:i], conns[i+1:]...)
				break
			}
		}
		if len(shard.byChannel[conn.Channel]) == 0 {
			delete(shard.byChannel, conn.Channel)
		}
	}
}

func (r *Registry) GetByUser(userKey string) *Connection {
	parts := strings.Split(userKey, ":")
	if len(parts) != 3 {
		return nil
	}
	channel := parts[0]

	shard := r.getShard(channel)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	return shard.byUser[userKey]
}

func (r *Registry) GetByGroup(channel, group string) []*Connection {
	shard := r.getShard(channel)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	groupKey := channel + ":" + group
	conns := shard.byGroup[groupKey]
	result := make([]*Connection, len(conns))
	copy(result, conns)
	return result
}

func (r *Registry) GetByChannel(channel string) []*Connection {
	shard := r.getShard(channel)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	conns := shard.byChannel[channel]
	result := make([]*Connection, len(conns))
	copy(result, conns)
	return result
}

func (r *Registry) GetAll() []*Connection {
	var result []*Connection
	for _, shard := range r.shards {
		shard.mu.RLock()
		for _, conn := range shard.byUser {
			result = append(result, conn)
		}
		shard.mu.RUnlock()
	}
	return result
}

func (r *Registry) Stats() (connections, groups, channels int) {
	for _, shard := range r.shards {
		shard.mu.RLock()
		connections += len(shard.byUser)
		groups += len(shard.byGroup)
		channels += len(shard.byChannel)
		shard.mu.RUnlock()
	}
	return
}
