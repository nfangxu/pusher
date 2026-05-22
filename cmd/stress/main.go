package main

import (
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "connect":
		runConnectCmd()
	case "push":
		runPushCmd()
	case "stability":
		runStabilityCmd()
	default:
		fmt.Fprintf(os.Stderr, "未知命令: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("用法: stress <command> [options]")
	fmt.Println()
	fmt.Println("命令:")
	fmt.Println("  connect     SSE 连接建立压测")
	fmt.Println("  push        消息推送压测")
	fmt.Println("  stability   高并发稳定性压测")
	fmt.Println()
	fmt.Println("示例:")
	fmt.Println("  stress connect --num 10000 --concurrency 1000 --salt mysecret")
	fmt.Println("  stress push --num 1000 --concurrency 100 --salt mysecret --token pushtoken")
	fmt.Println("  stress stability --num 100000 --duration 30m --salt mysecret --token pushtoken")
}

func runConnectCmd() {
	fs := flag.NewFlagSet("connect", flag.ExitOnError)
	url := fs.String("url", "http://localhost:8080", "服务地址")
	salt := fs.String("salt", "", "Token 签名盐值（必填）")
	num := fs.Int("num", 10000, "连接数")
	concurrency := fs.Int("concurrency", 1000, "并发数")
	fs.Parse(os.Args[2:])

	if *salt == "" {
		fmt.Fprintln(os.Stderr, "错误: --salt 为必填参数")
		fs.Usage()
		os.Exit(1)
	}

	runConnect(*url, *salt, *num, *concurrency)
}

func runPushCmd() {
	fs := flag.NewFlagSet("push", flag.ExitOnError)
	url := fs.String("url", "http://localhost:8080", "服务地址")
	salt := fs.String("salt", "", "Token 签名盐值（必填）")
	pushToken := fs.String("token", "", "推送接口 Token（必填）")
	num := fs.Int("num", 1000, "推送请求数")
	concurrency := fs.Int("concurrency", 100, "并发数")
	target := fs.String("target", "*", "推送目标")
	fs.Parse(os.Args[2:])

	if *salt == "" || *pushToken == "" {
		fmt.Fprintln(os.Stderr, "错误: --salt 和 --token 为必填参数")
		fs.Usage()
		os.Exit(1)
	}

	runPush(*url, *salt, *pushToken, *target, *num, *concurrency)
}

func runStabilityCmd() {
	fs := flag.NewFlagSet("stability", flag.ExitOnError)
	url := fs.String("url", "http://localhost:8080", "服务地址")
	salt := fs.String("salt", "", "Token 签名盐值（必填）")
	pushToken := fs.String("token", "", "推送接口 Token（必填）")
	num := fs.Int("num", 100000, "连接数")
	duration := fs.Duration("duration", 30*time.Minute, "压测时长")
	qps := fs.Int("qps", 500, "推送 QPS")
	fs.Parse(os.Args[2:])

	if *salt == "" || *pushToken == "" {
		fmt.Fprintln(os.Stderr, "错误: --salt 和 --token 为必填参数")
		fs.Usage()
		os.Exit(1)
	}

	runStability(*url, *salt, *pushToken, *num, *duration, *qps)
}
