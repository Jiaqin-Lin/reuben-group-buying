package redisx

import (
	"context"
	"time"
)

// newTestContext 返回测试用的 context（3 秒超时）。
func newTestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 3*time.Second)
}
