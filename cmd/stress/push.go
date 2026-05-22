package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"pusher/internal/token"
)

func runPush(url, salt, pushToken, target string, num, concurrency int) {
	fmt.Printf("开始推送压测: url=%s target=%s num=%d concurrency=%d\n", url, target, num, concurrency)

	// 建立在线连接
	connCount := concurrency * 10
	if connCount > 1000 {
		connCount = 1000
	}
	fmt.Printf("建立 %d 个在线连接...\n", connCount)
	holdConnections(url, salt, connCount)
	time.Sleep(500 * time.Millisecond)

	// 推送压测
	stats := NewStats("推送压测")
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	client := &http.Client{Timeout: 5 * time.Second}
	var seq atomic.Int64

	for i := 0; i < num; i++ {
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
	client := &http.Client{Timeout: 10 * time.Second}
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			channel := fmt.Sprintf("bench%d", idx%10)
			group := fmt.Sprintf("g%d", idx%5)
			uuid := fmt.Sprintf("u%d", idx)
			tk := token.Generate(salt, channel, group, uuid)

			req, _ := http.NewRequest("GET", url+"/sse/connect?token="+tk, nil)
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			// 保持连接不关闭
			go func() {
				buf := make([]byte, 1)
				for {
					if _, err := resp.Body.Read(buf); err != nil {
						return
					}
				}
			}()
		}(i)
	}
	wg.Wait()
}
