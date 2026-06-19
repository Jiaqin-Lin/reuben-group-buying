package dynamic

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ==================== Def[T] 单元测试（无外部依赖） ====================

func TestDef_GetDefault(t *testing.T) {
	d := NewDef("test.key", 42, "test config")
	if d.Get() != 42 {
		t.Errorf("Get: want 42, got %d", d.Get())
	}
}

func TestDef_GetAfterSet(t *testing.T) {
	d := NewDef("test.key", 42, "test config")
	d.loadFromJSON("99")
	if d.Get() != 99 {
		t.Errorf("Get: want 99, got %d", d.Get())
	}
}

func TestDef_LoadFromJSON_Int(t *testing.T) {
	d := NewDef("test.key", 42, "test config")
	if err := d.loadFromJSON("99"); err != nil {
		t.Fatalf("loadFromJSON: %v", err)
	}
	if d.Get() != 99 {
		t.Errorf("Get after load: want 99, got %d", d.Get())
	}
}

func TestDef_LoadFromJSON_Bool(t *testing.T) {
	d := NewDef("test.key", false, "feature flag")
	if err := d.loadFromJSON("true"); err != nil {
		t.Fatalf("loadFromJSON: %v", err)
	}
	if d.Get() != true {
		t.Errorf("Get: want true, got %v", d.Get())
	}
}

func TestDef_LoadFromJSON_Bool_False(t *testing.T) {
	d := NewDef("test.key", true, "feature flag")
	if err := d.loadFromJSON("false"); err != nil {
		t.Fatalf("loadFromJSON: %v", err)
	}
	if d.Get() != false {
		t.Errorf("Get: want false, got %v", d.Get())
	}
}

func TestDef_LoadFromJSON_String(t *testing.T) {
	d := NewDef("test.key", "default", "string config")
	if err := d.loadFromJSON(`"hello world"`); err != nil {
		t.Fatalf("loadFromJSON: %v", err)
	}
	if d.Get() != "hello world" {
		t.Errorf("Get: want 'hello world', got %q", d.Get())
	}
}

func TestDef_LoadFromJSON_Invalid(t *testing.T) {
	d := NewDef("test.key", 42, "int config")
	err := d.loadFromJSON(`"not a number"`)
	if err == nil {
		t.Fatal("loadFromJSON: want error for invalid JSON, got nil")
	}
	// Default should be preserved
	if d.Get() != 42 {
		t.Errorf("Get after failed load: want 42, got %d", d.Get())
	}
}

func TestDef_ConcurrentAccess(t *testing.T) {
	d := NewDef("test.key", 0, "concurrent test")

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			d.val.Store(i)
		}
		close(done)
	}()

	for i := 0; i < 1000; i++ {
		_ = d.Get() // 读不应 panic
	}
	<-done
}

// ==================== configDef 接口 ====================

func TestConfigDef_Interface(t *testing.T) {
	d := NewDef("my.key", 100, "test")
	var cd configDef = d // 编译期验证实现接口

	if cd.getKey() != "my.key" {
		t.Errorf("getKey: want 'my.key', got %q", cd.getKey())
	}
	if cd.getDefault() != 100 {
		t.Errorf("getDefault: want 100, got %v", cd.getDefault())
	}
	if cd.getValue() != 100 {
		t.Errorf("getValue: want 100, got %v", cd.getValue())
	}
}

// ==================== Manager 集成测试（需要 Docker MySQL + Redis） ====================

func testManager(t *testing.T) (*Manager, func()) {
	t.Helper()

	// MySQL
	dsn := "dev:dev123@tcp(127.0.0.1:3306)/group_buy_market?charset=utf8mb4&parseTime=True&loc=UTC"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Skip("mysql not available")
	}
	sqlDB, _ := db.DB()
	if sqlDB == nil || sqlDB.Ping() != nil {
		t.Skip("mysql ping failed")
	}

	// Redis
	rdb := goredis.NewClient(&goredis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       2, // 用 DB 2 避免冲突
	})
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		sqlDB.Close()
		t.Skip("redis not available")
	}

	// 清空测试数据
	db.Exec("DELETE FROM dynamic_configs WHERE config_key LIKE 'test.%'")
	rdb.FlushDB(ctx).Err()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	mgr := New(db, rdb, "bgm:config:updates:test", logger)

	cleanup := func() {
		db.Exec("DELETE FROM dynamic_configs WHERE config_key LIKE 'test.%'")
		rdb.FlushDB(ctx).Err()
		mgr.Stop()
		sqlDB.Close()
		rdb.Close()
	}

	return mgr, cleanup
}

func TestManager_RegisterAndLoad(t *testing.T) {
	mgr, cleanup := testManager(t)
	defer cleanup()

	d := NewDef("test.load_key", 123, "test load config")

	if err := mgr.Register(d); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// 第一次 Load，MySQL 无记录 → 应写入默认值
	if d.Get() != 123 {
		t.Errorf("Get after Load: want 123, got %d", d.Get())
	}

	// 验证 MySQL 中已写入
	var row DynamicConfig
	if err := mgr.db.Where("config_key = ?", "test.load_key").First(&row).Error; err != nil {
		t.Fatalf("find in mysql: %v", err)
	}
	if row.ConfigValue != "123" {
		t.Errorf("config_value in mysql: want '123', got %q", row.ConfigValue)
	}
	if row.Version != 1 {
		t.Errorf("version: want 1, got %d", row.Version)
	}
}

func TestManager_LoadFromExisting(t *testing.T) {
	mgr, cleanup := testManager(t)
	defer cleanup()

	// 先手动写入 MySQL
	if err := mgr.db.Create(&DynamicConfig{
		ConfigKey:   "test.existing_key",
		ConfigValue: "456",
		Version:     2,
	}).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}

	d := NewDef("test.existing_key", 999, "test existing config")
	if err := mgr.Register(d); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// 应从 MySQL 读，忽略 Default
	if d.Get() != 456 {
		t.Errorf("Get: want 456 (from MySQL), got %d", d.Get())
	}
}

func TestManager_Set(t *testing.T) {
	mgr, cleanup := testManager(t)
	defer cleanup()

	d := NewDef("test.set_key", 0, "test set config")
	if err := mgr.Register(d); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Set 新值
	if err := mgr.Set(context.Background(), "test.set_key", 789, "tester"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// 本地应更新
	if d.Get() != 789 {
		t.Errorf("Get after Set: want 789, got %d", d.Get())
	}

	// MySQL 应更新
	var row DynamicConfig
	if err := mgr.db.Where("config_key = ?", "test.set_key").First(&row).Error; err != nil {
		t.Fatalf("find in mysql: %v", err)
	}
	if row.ConfigValue != "789" {
		t.Errorf("mysql value: want '789', got %q", row.ConfigValue)
	}
	if row.UpdatedBy != "tester" {
		t.Errorf("updated_by: want 'tester', got %q", row.UpdatedBy)
	}
}

func TestManager_Set_UnknownKey(t *testing.T) {
	mgr, cleanup := testManager(t)
	defer cleanup()

	err := mgr.Set(context.Background(), "test.nonexistent", 1, "tester")
	if err == nil {
		t.Fatal("Set unknown key: want error, got nil")
	}
}

func TestManager_RegisterDuplicate(t *testing.T) {
	mgr, cleanup := testManager(t)
	defer cleanup()

	d1 := NewDef("test.dup_key", 1, "first")
	d2 := NewDef("test.dup_key", 2, "second")

	if err := mgr.Register(d1); err != nil {
		t.Fatalf("Register first: %v", err)
	}
	if err := mgr.Register(d2); err == nil {
		t.Fatal("Register duplicate: want error, got nil")
	}
}

func TestManager_GetAll(t *testing.T) {
	mgr, cleanup := testManager(t)
	defer cleanup()

	d1 := NewDef("test.all_1", 10, "first")
	d2 := NewDef("test.all_2", "hello", "second")

	if err := mgr.Register(d1, d2); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	all := mgr.GetAll()
	if len(all) != 2 {
		t.Errorf("GetAll len: want 2, got %d", len(all))
	}
}

func TestManager_BoolConfig(t *testing.T) {
	mgr, cleanup := testManager(t)
	defer cleanup()

	d := NewDef("test.bool_key", false, "bool config")
	if err := mgr.Register(d); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := mgr.Load(context.Background()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if d.Get() != false {
		t.Errorf("initial: want false, got %v", d.Get())
	}

	// Set true
	if err := mgr.Set(context.Background(), "test.bool_key", true, "tester"); err != nil {
		t.Fatalf("Set true: %v", err)
	}
	if d.Get() != true {
		t.Errorf("after Set true: want true, got %v", d.Get())
	}

	// Set false
	if err := mgr.Set(context.Background(), "test.bool_key", false, "tester"); err != nil {
		t.Fatalf("Set false: %v", err)
	}
	if d.Get() != false {
		t.Errorf("after Set false: want false, got %v", d.Get())
	}
}

func TestManager_Watch(t *testing.T) {
	mgr1, cleanup1 := testManager(t)
	defer cleanup1()
	mgr2, cleanup2 := testManager(t)
	defer cleanup2()

	d1 := NewDef("test.watch_key", 0, "watch test")
	d2 := NewDef("test.watch_key", 0, "watch test")

	if err := mgr1.Register(d1); err != nil {
		t.Fatalf("mgr1 Register: %v", err)
	}
	if err := mgr2.Register(d2); err != nil {
		t.Fatalf("mgr2 Register: %v", err)
	}
	if err := mgr1.Load(context.Background()); err != nil {
		t.Fatalf("mgr1 Load: %v", err)
	}
	if err := mgr2.Load(context.Background()); err != nil {
		t.Fatalf("mgr2 Load: %v", err)
	}

	// mgr2 开始 Watch（模拟另一个实例）
	mgr2.Watch(context.Background())

	// mgr1 Set 新值 → mgr2 应收到通知并更新
	if err := mgr1.Set(context.Background(), "test.watch_key", 42, "tester"); err != nil {
		t.Fatalf("mgr1 Set: %v", err)
	}

	// 给 Pub/Sub 一点时间传播
	time.Sleep(100 * time.Millisecond)

	if d2.Get() != 42 {
		t.Errorf("mgr2 after watch: want 42, got %d", d2.Get())
	}
}

// ==================== notifyMsg 序列化 ====================

func TestNotifyMsg_JSON(t *testing.T) {
	msg := notifyMsg{Key: "trial.cache_ttl"}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != `{"key":"trial.cache_ttl"}` {
		t.Errorf("json: want '{\"key\":\"trial.cache_ttl\"}', got %q", string(data))
	}

	var parsed notifyMsg
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Key != "trial.cache_ttl" {
		t.Errorf("parsed key: want 'trial.cache_ttl', got %q", parsed.Key)
	}
}
