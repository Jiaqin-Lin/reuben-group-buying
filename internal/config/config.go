// Package config 全局配置加载（Viper：YAML + 环境变量）。
// 环境变量自动映射：DB_HOST → mysql.host，DB_PORT → mysql.port 等。
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config 全局配置
type Config struct {
	Server ServerConfig `mapstructure:"server"`
	MySQL  MySQLConfig  `mapstructure:"mysql"`
	Redis  RedisConfig  `mapstructure:"redis"`
	Log    LogConfig    `mapstructure:"log"`
	App    AppConfig    `mapstructure:"app"`
	Alipay AlipayConfig `mapstructure:"alipay"`
}

// ServerConfig HTTP 服务配置
type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	Mode         string        `mapstructure:"mode"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

// MySQLConfig 数据库连接配置
type MySQLConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	User            string        `mapstructure:"user"`
	Password        string        `mapstructure:"password"`
	Database        string        `mapstructure:"database"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// DSN 构建 MySQL 连接字符串
func (c MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User, c.Password, c.Host, c.Port, c.Database)
}

// RedisConfig Redis 连接配置
type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"` // text | json
}

// AppConfig 业务配置
type AppConfig struct {
	Name                string `mapstructure:"name"`
	LockResultTTL       int    `mapstructure:"lock_result_ttl"`
	OrderLockTTL        int    `mapstructure:"order_lock_ttl"`
	NotifyMaxRetry      int    `mapstructure:"notify_max_retry"`
	TimeoutScanInterval int    `mapstructure:"timeout_scan_interval"`
	AdminToken          string `mapstructure:"admin_token"`
}

// AlipayConfig 支付宝支付配置。
// 不配置时（app_id 为空）自动降级使用 Mock 支付网关。
type AlipayConfig struct {
	AppID           string `mapstructure:"app_id"`            // 沙箱/正式应用 APPID
	PrivateKey      string `mapstructure:"private_key"`       // 应用私钥（PEM 内容或文件路径）
	AlipayPublicKey string `mapstructure:"alipay_public_key"` // 支付宝公钥（用于验签回调）
	NotifyURL       string `mapstructure:"notify_url"`        // 异步回调地址（支付宝 POST 通知到此）
	ReturnURL       string `mapstructure:"return_url"`        // 同步跳转地址（可选）
	Sandbox         bool   `mapstructure:"sandbox"`           // true=沙箱, false=正式
	SignType        string `mapstructure:"sign_type"`         // RSA2（默认）
}

// Load 从指定路径加载配置。path 为空时默认搜索当前目录和 ./config/。
func Load(path string) (*Config, error) {
	v := viper.New()

	// 配置文件
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("../") // 从 cmd/server 运行时

	if path != "" {
		v.SetConfigFile(path)
	}

	// 环境变量映射（DB_HOST → mysql.host）
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// 默认值
	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
		// 配置文件不存在时使用默认值 + 环境变量
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.mode", "debug")
	v.SetDefault("server.read_timeout", "10s")
	v.SetDefault("server.write_timeout", "10s")

	v.SetDefault("mysql.host", "127.0.0.1")
	v.SetDefault("mysql.port", 3306)
	v.SetDefault("mysql.max_open_conns", 50)
	v.SetDefault("mysql.max_idle_conns", 10)
	v.SetDefault("mysql.conn_max_lifetime", "300s")

	v.SetDefault("redis.addr", "127.0.0.1:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	v.SetDefault("log.level", "debug")
	v.SetDefault("log.format", "text")

	v.SetDefault("app.name", "group-buy")
	v.SetDefault("app.lock_result_ttl", 600)
	v.SetDefault("app.order_lock_ttl", 15)
	v.SetDefault("app.notify_max_retry", 5)
	v.SetDefault("app.timeout_scan_interval", 30)
	v.SetDefault("app.admin_token", "admin-dev-token")

	v.SetDefault("alipay.sandbox", true)
	v.SetDefault("alipay.sign_type", "RSA2")
}
