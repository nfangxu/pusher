package main

import (
	"fmt"
	"net"
	"sync"
	"time"

	"pusher/internal/token"
)

func runConnect(url, salt string, num, concurrency int) {
	fmt.Printf("开始连接压测: url=%s num=%d concurrency=%d\n", url, num, concurrency)

	stats := NewStats("连接压测")
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := 0; i < num; i++ {
		wg.Add(1)
		sem <- struct{}{}

		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			channel := fmt.Sprintf("ch%d", idx%10)
			group := fmt.Sprintf("g%d", idx%5)
			uuid := fmt.Sprintf("u%d", idx)
			tk := token.Generate(salt, channel, group, uuid)

			start := time.Now()
			ok := doConnect(url, tk)
			stats.Record(ok, time.Since(start))
		}(i)
	}

	wg.Wait()
	stats.Report()
}

func doConnect(url, tk string) bool {
	conn, err := net.DialTimeout("tcp", url[len("http://"):], 3*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(5 * time.Second))

	req := fmt.Sprintf("GET /sse/connect?token=%s HTTP/1.1\r\nHost: localhost\r\n\r\n", tk)
	if _, err := conn.Write([]byte(req)); err != nil {
		return false
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || n == 0 {
		return false
	}

	return len(buf) > 12 && string(buf[9:12]) == "200"
}
