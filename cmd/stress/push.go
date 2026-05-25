package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"pusher/internal/token"
)

func runPush(url, salt, pushToken, target string, num, concurrency, qps int) {
	fmt.Printf("开始推送压测: url=%s target=%s num=%d concurrency=%d qps=%d\n", url, target, num, concurrency, qps)

	// 建立在线连接
	connCount := concurrency * 10
	if connCount > 500 {
		connCount = 500
	}
	fmt.Printf("建立 %d 个在线连接...\n", connCount)
	holdConnections(url, salt, connCount)
	time.Sleep(500 * time.Millisecond)

	// 推送压测
	stats := NewStats("推送压测")
	client := &http.Client{Timeout: 5 * time.Second}
	var seq atomic.Int64
	var wg sync.WaitGroup

	// 用 ticker 控制发送速率
	ticker := time.NewTicker(time.Second / time.Duration(qps))
	defer ticker.Stop()

	sem := make(chan struct{}, concurrency)

	for i := 0; i < num; i++ {
		<-ticker.C
		wg.Add(1)
		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			msg := map[string]interface{}{
				"type": "bench",
				"seq":  seq.Add(1),
			}
			start := time.Now()
			ok := doPush(client, url, pushToken, []string{target}, msg)
			stats.Record(ok, time.Since(start))
		}()
	}

	wg.Wait()
	stats.Report()
}

func doPush(client *http.Client, url, pushToken string, targets []string, message interface{}) bool {
	body, _ := json.Marshal(map[string]interface{}{
		"targets": targets,
		"message": message,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", url+"/push", bytes.NewReader(body))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+pushToken)

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var result struct {
		Code int `json:"code"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.Code == 0
}

func holdConnections(url, salt string, count int) {
	var wg sync.WaitGroup
	addr := url[len("http://"):]

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			channel := fmt.Sprintf("bench%d", idx%10)
			group := fmt.Sprintf("g%d", idx%5)
			uuid := fmt.Sprintf("u%d", idx)
			tk := token.Generate(salt, channel, group, uuid)

			conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
			if err != nil {
				return
			}

			req := fmt.Sprintf("GET /sse/connect?token=%s HTTP/1.1\r\nHost: localhost\r\n\r\n", tk)
			conn.Write([]byte(req))
			// 保持连接不关闭
			buf := make([]byte, 1)
			go func() {
				for {
					if _, err := conn.Read(buf); err != nil {
						return
					}
				}
			}()
		}(i)
	}
	wg.Wait()
}
