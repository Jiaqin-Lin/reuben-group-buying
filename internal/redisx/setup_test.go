package redisx

import (
	"os"
	"testing"

	goredis "github.com/redis/go-redis/v9"
)

// testRDB 在 TestMain 中初始化，所有测试共享一个 Redis 连接。
var testRDB *goredis.Client

// TestMain 统一初始化：连接 docker-compose Redis。
func TestMain(m *testing.M) {
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       0,
	})

	// 检查 Redis 是否可用
	ctx, cancel := newTestContext()
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		println("skip redisx tests: redis not available (run `docker compose up -d redis` first)")
		rdb.Close()
		return
	}

	testRDB = rdb

	// 清空 DB 0，确保测试隔离
	rdb.FlushDB(ctx).Err()

	code := m.Run()

	// 清理
	rdb.Close()
	os.Exit(code)
}

// flushTestDB 在每个测试前清空 Redis DB 0。
func flushTestDB(t *testing.T) {
	t.Helper()
	if testRDB == nil {
		t.Skip("redis not available")
	}
	ctx, cancel := newTestContext()
	defer cancel()
	if err := testRDB.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush test db: %v", err)
	}
}
