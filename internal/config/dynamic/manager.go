package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// configDef 内部接口，统一处理不同类型 Def[T] 的注册和加载。
type configDef interface {
	getKey() string
	getDefault() any
	getValue() any
	loadFromJSON(jsonStr string) error
	bind(*Manager)
}

// notifyMsg 配置变更通知的 Pub/Sub 载荷。
type notifyMsg struct {
	Key string `json:"key"`
}

// Manager 动态配置管理器。
//
// MySQL 做 source of truth，本地 atomic.Value 做热读，
// Redis Pub/Sub 做跨实例变更通知。
type Manager struct {
	db      *gorm.DB
	rdb     *redis.Client
	channel string
	logger  *slog.Logger

	defs map[string]configDef // key → Def
	mu   sync.RWMutex

	// watch 控制
	ctx    context.Context
	cancel context.CancelFunc
}

// New 创建 Manager。需要在 app 层显式调用 Register 注册配置项，然后 Load 从 MySQL 加载。
func New(db *gorm.DB, rdb *redis.Client, channel string, logger *slog.Logger) *Manager {
	return &Manager{
		db:      db,
		rdb:     rdb,
		channel: channel,
		logger:  logger,
		defs:    make(map[string]configDef),
	}
}

// Register 注册一组配置定义。
//
// 参数为 *Def[T] 指针（T 仅支持 int、bool、string）。
// 注册后 def 绑定到当前 Manager，Get() 调用返回当前值。
//
// 用法:
//
//	mgr.Register(TrialCacheTTL, NotifyMaxRetry, FeatureSkipCrowd)
func (m *Manager) Register(defs ...any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, d := range defs {
		cd, ok := d.(configDef)
		if !ok {
			return fmt.Errorf("dynamic.Register: type %T does not implement configDef", d)
		}
		key := cd.getKey()
		if _, exists := m.defs[key]; exists {
			return fmt.Errorf("dynamic.Register: duplicate key %q", key)
		}
		cd.bind(m)
		m.defs[key] = cd
	}
	return nil
}

// Load 从 MySQL 批量加载所有已注册配置项。
// 如果 MySQL 中不存在，使用 Default 值（并写入 MySQL 作为初始化）。
//
// 调用时机：Register 全部完成后、Watch 之前。
func (m *Manager) Load(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for key, cd := range m.defs {
		if err := m.loadOne(ctx, key, cd); err != nil {
			return fmt.Errorf("dynamic.Load key=%s: %w", key, err)
		}
	}
	return nil
}

// loadOne 从 MySQL 加载单个配置项，不存在则写入默认值。
func (m *Manager) loadOne(ctx context.Context, key string, cd configDef) error {
	var row DynamicConfig
	err := m.db.WithContext(ctx).
		Where("config_key = ?", key).
		First(&row).Error

	if err == nil {
		// MySQL 中有 → 用 MySQL 的值
		if loadErr := cd.loadFromJSON(row.ConfigValue); loadErr != nil {
			m.logger.Warn("dynamic: invalid JSON, using default", "key", key, "value", row.ConfigValue, "error", loadErr)
			// 格式错误时保留 Default，不写入 MySQL（避免覆盖正确的历史值）
			return nil
		}
		return nil
	}

	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("query: %w", err)
	}

	// MySQL 中无记录 → 写入默认值
	defJSON, _ := json.Marshal(cd.getDefault()) // 基础类型 JSON marshal 不会失败
	row = DynamicConfig{ConfigKey: key, ConfigValue: string(defJSON), Version: 1}
	if insertErr := m.db.WithContext(ctx).Create(&row).Error; insertErr != nil {
		m.logger.Warn("dynamic: failed to insert default, using default in memory", "key", key, "error", insertErr)
	}
	return nil
}

// Set 更新配置值。写 MySQL → 更新本地 → Publish 通知其他实例。
//
//   - key: 配置键
//   - value: 新值（int/bool/string，会被 JSON 序列化）
//   - updatedBy: 操作人标识
func (m *Manager) Set(ctx context.Context, key string, value any, updatedBy string) error {
	m.mu.RLock()
	cd, ok := m.defs[key]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("dynamic.Set: unknown key %q", key)
	}

	// 1. JSON 序列化新值
	rawJSON, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("dynamic.Set marshal: %w", err)
	}
	jsonStr := string(rawJSON)

	// 2. 写 MySQL（Upsert）
	result := m.db.WithContext(ctx).
		Model(&DynamicConfig{}).
		Where("config_key = ?", key).
		Updates(map[string]any{
			"config_value": jsonStr,
			"version":      gorm.Expr("version + 1"),
			"updated_by":   updatedBy,
			"updated_at":   time.Now(),
		})
	if result.Error != nil {
		return fmt.Errorf("dynamic.Set update mysql: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// 不应发生（Load 时已插入默认值），防御性 insert
		row := DynamicConfig{ConfigKey: key, ConfigValue: jsonStr, Version: 1, UpdatedBy: updatedBy}
		if err := m.db.WithContext(ctx).Create(&row).Error; err != nil {
			return fmt.Errorf("dynamic.Set insert mysql: %w", err)
		}
	}

	// 3. 更新本地
	if err := cd.loadFromJSON(jsonStr); err != nil {
		return fmt.Errorf("dynamic.Set load local: %w", err)
	}

	// 4. Publish 通知其他实例
	msg, _ := json.Marshal(notifyMsg{Key: key})
	if err := m.rdb.Publish(ctx, m.channel, string(msg)).Err(); err != nil {
		m.logger.Warn("dynamic.Set publish failed (local updated)", "key", key, "error", err)
	}

	m.logger.Info("dynamic config updated", "key", key, "version", "incremented", "by", updatedBy)
	return nil
}

// Watch 订阅 Redis Pub/Sub，收到配置变更通知后从 MySQL 重读该 key。
// 在 goroutine 中运行，不阻塞调用方。通过 ctx 控制生命周期。
//
// 调用时机：Load 完成后，在 app.Run() 中 goroutine 启动。
func (m *Manager) Watch(ctx context.Context) {
	m.ctx, m.cancel = context.WithCancel(ctx)

	go func() {
		pubsub := m.rdb.Subscribe(m.ctx, m.channel)
		defer pubsub.Close()

		ch := pubsub.Channel()
		m.logger.Info("dynamic config watch started", "channel", m.channel)

		for {
			select {
			case <-m.ctx.Done():
				m.logger.Info("dynamic config watch stopped")
				return
			case msg, ok := <-ch:
				if !ok {
					m.logger.Warn("dynamic config watch channel closed, reconnecting...")
					pubsub.Close()
					pubsub = m.rdb.Subscribe(m.ctx, m.channel)
					ch = pubsub.Channel()
					continue
				}
				m.handleNotify(msg.Payload)
			}
		}
	}()
}

// Stop 停止 Watch goroutine。
func (m *Manager) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
}

// handleNotify 处理来自 Pub/Sub 的配置变更通知。
func (m *Manager) handleNotify(payload string) {
	var msg notifyMsg
	if err := json.Unmarshal([]byte(payload), &msg); err != nil {
		m.logger.Warn("dynamic: invalid notify message", "payload", payload)
		return
	}

	m.mu.RLock()
	cd, ok := m.defs[msg.Key]
	m.mu.RUnlock()

	if !ok {
		// 其他实例可能有本实例未注册的 key，忽略
		return
	}

	// 从 MySQL 重读
	var row DynamicConfig
	err := m.db.WithContext(context.Background()).
		Where("config_key = ?", msg.Key).
		First(&row).Error
	if err != nil {
		m.logger.Warn("dynamic: failed to reload from mysql", "key", msg.Key, "error", err)
		return
	}

	if err := cd.loadFromJSON(row.ConfigValue); err != nil {
		m.logger.Warn("dynamic: invalid JSON on reload", "key", msg.Key, "value", row.ConfigValue)
		return
	}

	m.logger.Info("dynamic config reloaded", "key", msg.Key, "version", row.Version)
}

// GetAll 返回所有已注册配置项的当前值，供 admin API 使用。
func (m *Manager) GetAll() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]any, len(m.defs))
	for key, cd := range m.defs {
		out[key] = map[string]any{
			"value": cd.getValue(),
		}
	}
	return out
}

// DynamicConfig 仅用于内部 MySQL 操作，不暴露。
type DynamicConfig struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement"`
	ConfigKey   string    `gorm:"type:varchar(128);uniqueIndex;not null;column:config_key"`
	ConfigValue string    `gorm:"type:text;not null;column:config_value"`
	Version     uint      `gorm:"type:int unsigned;not null;default:1"`
	UpdatedBy   string    `gorm:"type:varchar(64);not null;default:'';column:updated_by"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}

// TableName 表名。
func (DynamicConfig) TableName() string { return "dynamic_configs" }
