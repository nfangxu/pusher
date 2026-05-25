package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"pusher/internal/token"
)

func main() {
	if len(os.Args) < 5 {
		fmt.Fprintf(os.Stderr, "用法: go run client.go <server_url> <salt> <channel> <group> <uuid>\n")
		fmt.Fprintf(os.Stderr, "示例: go run client.go http://localhost:8080 mysecret news admin u1\n")
		os.Exit(1)
	}

	serverURL := os.Args[1]
	salt := os.Args[2]
	channel := os.Args[3]
	group := os.Args[4]
	uuid := os.Args[5]

	tk := token.Generate(salt, channel, group, uuid)
	fmt.Printf("Token: %s\n", tk)
	fmt.Printf("连接: %s/sse/connect?token=%s\n\n", serverURL, tk)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := connect(ctx, serverURL, tk); err != nil {
		fmt.Fprintf(os.Stderr, "连接错误: %v\n", err)
		os.Exit(1)
	}
}

func connect(ctx context.Context, serverURL, tk string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", serverURL+"/sse/connect?token="+tk, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	fmt.Printf("已连接 (HTTP %d)\n\n", resp.StatusCode)

	scanner := bufio.NewScanner(resp.Body)
	var eventType string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			printMessage(eventType, data)
			eventType = ""
		} else if strings.HasPrefix(line, ":") {
			fmt.Printf("[心跳] %s\n", time.Now().Format("15:04:05"))
		}
	}

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			fmt.Println("\n已断开连接")
			return nil
		}
		return err
	}

	fmt.Println("\n连接已关闭")
	return nil
}

func printMessage(eventType, data string) {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(data), &parsed); err != nil {
		fmt.Printf("[%s] %s\n", eventType, data)
		return
	}

	channel, _ := parsed["channel"].(string)
	group, _ := parsed["group"].(string)
	uuid, _ := parsed["uuid"].(string)
	message := parsed["message"]

	msgJSON, _ := json.MarshalIndent(message, "  ", "  ")
	fmt.Printf("[%s] 收到消息 %s:%s:%s\n  %s\n\n",
		time.Now().Format("15:04:05"), channel, group, uuid, msgJSON)
}
