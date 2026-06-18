package repository

import (
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// testDB 在 TestMain 中初始化，所有测试共享一个连接。
var testDB *gorm.DB

// TestMain 统一初始化：连接 docker-compose MySQL，自动迁移表结构，插入种子数据。
func TestMain(m *testing.M) {
	// 使用 docker-compose 中的 MySQL（dev:dev123@127.0.0.1:3306）
	// loc=UTC：Docker MySQL 容器默认 UTC 时区，用 UTC 保证 Go 时间与 MySQL NOW() 一致
	dsn := "dev:dev123@tcp(127.0.0.1:3306)/group_buy_market?charset=utf8mb4&parseTime=True&loc=UTC"

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		// MySQL 不可用时跳过所有集成测试，不要 panic
		println("skip integration tests: mysql not available (run `docker compose up -d` first)")
		return
	}

	sqlDB, _ := db.DB()
	if sqlDB == nil || sqlDB.Ping() != nil {
		println("skip integration tests: mysql ping failed")
		return
	}
	defer sqlDB.Close()

	testDB = db

	// 清空旧数据（TRUNCATE 会重置 AUTO_INCREMENT，方便测试）
	truncateAll(testDB)

	// 插入种子数据（确保测试有基础数据可查）
	seedTestData(testDB)

	// 运行测试
	m.Run()
}

// truncateAll 清空所有表（按外键顺序，先子表后父表）。
// 使用 TRUNCATE 而非 DELETE，确保 AUTO_INCREMENT 重置，测试可预测。
func truncateAll(db *gorm.DB) {
	// 先禁用外键检查，TRUNCATE 子表再 TRUNCATE 父表
	db.Exec("SET FOREIGN_KEY_CHECKS = 0")
	tables := []string{
		"payment_logs", "payments", "notify_tasks",
		"orders", "teams",
		"activity_products", "products", "activities", "discounts",
		"crowd_tag_details", "crowd_tag_jobs", "crowd_tags",
	}
	for _, t := range tables {
		db.Exec("TRUNCATE TABLE " + t)
	}
	db.Exec("SET FOREIGN_KEY_CHECKS = 1")
}

// seedTestData 插入测试用基础数据。
func seedTestData(db *gorm.DB) {
	// 折扣
	db.Exec(`INSERT INTO discounts (discount_id, name, description, plan_type, expression) VALUES
		('D001', '直减20元', '新用户直减20', 'ZJ', '20'),
		('D002', '8折优惠', 'VIP 8折', 'ZK', '0.8')
		ON DUPLICATE KEY UPDATE name=name`)

	// 活动（activity_id=100123 生效中，activity_id=100456 已过期）
	db.Exec(`INSERT INTO activities (activity_id, name, discount_id, group_type, target_count, take_limit, valid_minutes, status, start_time, end_time) VALUES
		(100123, '测试拼团活动', 'D001', 0, 3, 5, 30, 1, '2025-01-01', '2029-12-31'),
		(100456, '已过期活动', 'D002', 1, 2, 1, 15, 2, '2020-01-01', '2020-12-31')
		ON DUPLICATE KEY UPDATE name=name`)

	// 商品
	db.Exec(`INSERT INTO products (goods_id, goods_name, original_price) VALUES
		('GOODS001', '测试商品', 100.00),
		('GOODS002', '第二件商品', 50.00)
		ON DUPLICATE KEY UPDATE goods_name=goods_name`)

	// 活动-商品映射
	db.Exec(`INSERT INTO activity_products (source, channel, goods_id, activity_id) VALUES
		('APP', 'WECHAT', 'GOODS001', 100123)
		ON DUPLICATE KEY UPDATE activity_id=activity_id`)

	// 人群标签
	db.Exec(`INSERT INTO crowd_tags (tag_id, tag_name, tag_desc, statistics) VALUES
		('TAG001', 'VIP用户', 'VIP等级>=3', 100)
		ON DUPLICATE KEY UPDATE tag_name=tag_name`)
	db.Exec(`INSERT INTO crowd_tag_details (tag_id, user_id) VALUES
		('TAG001', 'USER999')
		ON DUPLICATE KEY UPDATE user_id=user_id`)
}
