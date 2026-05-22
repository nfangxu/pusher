package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"pusher/internal/token"
)

func runConnect(url, salt string, num, concurrency int) {
	fmt.Printf("开始连接压测: url=%s num=%d concurrency=%d\n", url, num, concurrency)

	stats := NewStats("连接压测")
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	client := &http.Client{Timeout: 10 * time.Second}

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
			ok := doConnect(client, url, tk)
			stats.Record(ok, time.Since(start))
		}(i)
	}

	wg.Wait()
	stats.Report()
}

func doConnect(client *http.Client, url, tk string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url+"/sse/connect?token="+tk, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	reader := bufio.NewReader(resp.Body)
	_, err = reader.ReadString('\n')
	return err == nil
}
