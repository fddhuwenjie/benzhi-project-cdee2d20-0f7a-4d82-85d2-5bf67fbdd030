package main

import (
	"context"
	"log"
	"os"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	cfg, err := parseConfig(os.Args[1:])
	if err != nil {
		log.Printf("配置错误: %v", err)
		os.Exit(2)
	}
	if err := run(context.Background(), cfg); err != nil {
		log.Printf("服务退出: %v", err)
		os.Exit(1)
	}
}
