// Package main 拼团营销系统入口。
package main

import (
	"fmt"
	"os"

	"github.com/reuben/group-buying/internal/app"
	"github.com/reuben/group-buying/internal/config"
)

func main() {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	a, err := app.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init app: %v\n", err)
		os.Exit(1)
	}

	if err := a.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "run app: %v\n", err)
		os.Exit(1)
	}
}
