// Package dynamic 动态配置系统。
//
// # 设计
//
// 对标字节 TCC 平台的使用体验：定义 key → 写 JSON → 代码中 struct 字段自动更新。
// Go 版实现：MySQL 做 source of truth + Redis Pub/Sub 做跨实例通知 + atomic.Value 做本地热读。
//
// 使用方式:
//
//  1. 在 defs.go 中定义配置项:
//     var TrialCacheTTL = NewDef[int]("trial.cache_ttl", 10, "试算缓存TTL（秒）")
//
//  2. 在 app.go 中注册:
//     mgr.Register(TrialCacheTTL, NotifyMaxRetry, ...)
//
//  3. 在业务代码中读取（无锁，热路径安全）:
//     ttl := TrialCacheTTL.Get()
//
//  4. 通过 admin API 更新（即时生效，所有实例同步）:
//     PUT /api/v1/admin/configs/trial.cache_ttl  {"value": 30}
//
// 支持的类型:
//   - Def[int]    整数配置（TTL、阈值、次数等）
//   - Def[bool]   开关配置（功能降级、feature flag）
//   - Def[string] 字符串配置（复杂 JSON、URL 模板等）
package dynamic

import (
	"encoding/json"
	"sync/atomic"
)

// Def 类型安全的动态配置定义。
//
// 零值不可用，必须通过 NewDef 创建并 Register 到 Manager 后才能 Get。
// Register 之前调用 Get 返回 Default。
type Def[T any] struct {
	Key     string // 配置键，同时也是 MySQL dynamic_configs.config_key 和 Redis 通知载荷
	Default T      // 硬编码默认值，启动时若 MySQL 无此 key 则使用
	Desc    string // 人类可读的描述

	mgr *Manager       // 注册后回填
	val *atomic.Value  // 当前值，Register 时初始化
}

// NewDef 创建一个配置定义。
// 仅在 defs.go 中调用，不要在业务代码中创建。
func NewDef[T any](key string, def T, desc string) *Def[T] {
	var v atomic.Value
	v.Store(def)
	return &Def[T]{
		Key:     key,
		Default: def,
		Desc:    desc,
		val:     &v,
	}
}

// Get 返回当前配置值。无锁，热路径安全。
//
//	ttl := TrialCacheTTL.Get()  // ~20ns，比读 Redis (~0.5ms) 快 25000 倍
func (d *Def[T]) Get() T {
	return d.val.Load().(T)
}

// --- configDef 接口实现 ---
// 这些方法仅供 Manager 内部使用，不要直接调用。

func (d *Def[T]) getKey() string     { return d.Key }
func (d *Def[T]) getDefault() any    { return d.Default }
func (d *Def[T]) getValue() any      { return d.val.Load() }
func (d *Def[T]) bind(m *Manager)    { d.mgr = m }

func (d *Def[T]) loadFromJSON(jsonStr string) error {
	var v T
	if err := json.Unmarshal([]byte(jsonStr), &v); err != nil {
		return err
	}
	d.val.Store(v)
	return nil
}
