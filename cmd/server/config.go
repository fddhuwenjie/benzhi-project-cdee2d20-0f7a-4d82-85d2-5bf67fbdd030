package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
)

const defaultAddress = "127.0.0.1:19081"

type config struct {
	address      string
	databasePath string
	selfCheck    bool
}

func parseConfig(args []string) (config, error) {
	portAddress := defaultAddress
	if value := os.Getenv("PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil || port < 1024 || port > 65535 {
			return config{}, fmt.Errorf("PORT 必须是 1024 到 65535 之间的端口号")
		}
		portAddress = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}
	flags := flag.NewFlagSet("洞潜任务安全放行服务", flag.ContinueOnError)
	address := flags.String("addr", portAddress, "HTTP 监听地址")
	database := flags.String("db", "dive_missions.db", "SQLite 数据库路径")
	selfCheck := flags.Bool("self-check", false, "执行真实 HTTP 全流程自检后退出")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, fmt.Errorf("存在无法识别的位置参数")
	}
	if err := validateAddress(*address); err != nil {
		return config{}, err
	}
	if *database == "" {
		return config{}, fmt.Errorf("数据库路径不能为空")
	}
	return config{address: *address, databasePath: *database, selfCheck: *selfCheck}, nil
}

func validateAddress(address string) error {
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("addr 格式错误: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("addr 必须使用回环 IP 地址")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1024 || port > 65535 {
		return fmt.Errorf("addr 端口必须在 1024 到 65535 之间")
	}
	return nil
}
