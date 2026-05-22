package main

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

func runStability(url, salt, pushToken string, connNum int, duration time.Duration, qps int) {
	fmt.Printf("开始稳定性压测: url=%s connections=%d duration=%s qps=%d\n", url, connNum, duration, qps)

	// 建立在线连接
	fmt.Printf("建立 %d 个在线连接...\n", connNum)
	holdConnections(url, salt, connNum)
	time.Sleep(500 * time.Millisecond)

	// 持续推送
	stats := NewStats("稳定性压测")
	client := &http.Client{Timeout: 5 * time.Second}

	var totalPush atomic.Int64
	var stop atomic.Bool

	// 打印快照
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for !stop.Load() {
			<-ticker.C
			fmt.Printf("[快照] 总推送=%d 成功=%d 失败=%d\n",
				stats.TotalCount(), stats.SuccessCount(), stats.FailCount())
		}
	}()

	// 推送循环
	interval := time.Second / time.Duration(qps)
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			stop.Store(true)
			stats.Report()
			return
		default:
		}

		msg := map[string]interface{}{
			"type": "stability",
			"seq":  totalPush.Add(1),
			"ts":   time.Now().Unix(),
		}

		start := time.Now()
		ok := doPush(client, url, pushToken, []string{"*"}, msg)
		stats.Record(ok, time.Since(start))

		time.Sleep(interval)
	}
}
