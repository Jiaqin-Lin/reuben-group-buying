-- 拼团营销系统 - 初始化建表
-- MySQL 8.0+
-- 13 张表：7 核心 + 2 支付 + 3 人群标签 + 1 动态配置

CREATE DATABASE IF NOT EXISTS `group_buy_market`
  DEFAULT CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;

USE `group_buy_market`;

-- ============================================================
-- 1. discounts — 折扣规则
-- ============================================================
CREATE TABLE `discounts` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `discount_id`   VARCHAR(16)     NOT NULL COMMENT '业务折扣ID',
  `name`          VARCHAR(64)     NOT NULL COMMENT '折扣名称',
  `description`   VARCHAR(256)    NOT NULL DEFAULT '' COMMENT '折扣描述',
  `plan_type`     VARCHAR(4)      NOT NULL DEFAULT 'ZJ' COMMENT 'ZJ=直减 MJ=满减 ZK=折扣 N=N元购',
  `expression`    VARCHAR(32)     NOT NULL COMMENT '计算表达式，如 20（直减20）、100-20（满100减20）、0.8（8折）、9.9（9.9元购）',
  `discount_type` TINYINT         NOT NULL DEFAULT 0 COMMENT '0=基础折扣 1=人群标签折扣',
  `tag_id`        VARCHAR(32)     DEFAULT NULL COMMENT '人群标签ID -> crowd_tags.tag_id（discount_type=1时生效）',
  `created_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_discount_id` (`discount_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='折扣规则';

-- ============================================================
-- 2. activities — 拼团活动
-- ============================================================
CREATE TABLE `activities` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `activity_id`   BIGINT          NOT NULL COMMENT '业务活动ID',
  `name`          VARCHAR(128)    NOT NULL COMMENT '活动名称',
  `discount_id`   VARCHAR(16)     NOT NULL COMMENT '关联折扣ID -> discounts.discount_id',
  `group_type`    TINYINT         NOT NULL DEFAULT 0 COMMENT '0=自动成团 1=目标拼团',
  `target_count`  INT             NOT NULL DEFAULT 1 COMMENT '成团所需人数',
  `take_limit`    INT             NOT NULL DEFAULT 1 COMMENT '用户参与次数上限',
  `valid_minutes` INT             NOT NULL DEFAULT 5 COMMENT '拼团有效时长（分钟）',
  `status`        TINYINT         NOT NULL DEFAULT 0 COMMENT '0=创建 1=生效 2=过期 3=废弃',
  `start_time`    DATETIME        NOT NULL COMMENT '活动开始时间',
  `end_time`      DATETIME        NOT NULL COMMENT '活动结束时间',
  `tag_id`        VARCHAR(32)     DEFAULT NULL COMMENT '人群标签ID -> crowd_tags.tag_id',
  `tag_scope`     VARCHAR(4)      DEFAULT NULL COMMENT '标签范围；第1位=可见限制 第2位=参与限制（1=限制 0=不限制）',
  `created_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_activity_id` (`activity_id`),
  KEY `idx_status_time` (`status`, `start_time`, `end_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='拼团活动';

-- ============================================================
-- 3. products — 商品
-- ============================================================
CREATE TABLE `products` (
  `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `goods_id`       VARCHAR(32)     NOT NULL COMMENT '商品ID（全局唯一）',
  `goods_name`     VARCHAR(256)    NOT NULL COMMENT '商品名称',
  `original_price` DECIMAL(10,2)   NOT NULL COMMENT '商品原价',
  `created_at`     DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`     DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_goods_id` (`goods_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='商品';

-- ============================================================
-- 4. activity_products — 商品-活动映射（试算入口）
-- ============================================================
CREATE TABLE `activity_products` (
  `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `source`      VARCHAR(16)     NOT NULL COMMENT '来源（营销维度）',
  `channel`     VARCHAR(16)     NOT NULL COMMENT '渠道（营销维度）',
  `goods_id`    VARCHAR(32)     NOT NULL COMMENT '商品ID -> products.goods_id',
  `activity_id` BIGINT          NOT NULL COMMENT '活动ID -> activities.activity_id',
  `created_at`  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`  DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_source_channel_goods` (`source`, `channel`, `goods_id`),
  KEY `idx_activity_id` (`activity_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='商品-活动映射（试算入口）';

-- ============================================================
-- 5. teams — 拼团队伍
-- ============================================================
CREATE TABLE `teams` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `team_id`         VARCHAR(16)     NOT NULL COMMENT '团ID（8位随机数）',
  `activity_id`     BIGINT          NOT NULL COMMENT '活动ID -> activities.activity_id',
  `source`          VARCHAR(16)     NOT NULL COMMENT '来源（首单快照）',
  `channel`         VARCHAR(16)     NOT NULL COMMENT '渠道（首单快照）',
  `original_price`  DECIMAL(10,2)   NOT NULL COMMENT '商品原价（首单快照）',
  `deduction_price` DECIMAL(10,2)   NOT NULL COMMENT '折扣金额（首单快照）',
  `pay_price`       DECIMAL(10,2)   NOT NULL COMMENT '实付金额（首单快照）',
  `target_count`    INT             NOT NULL COMMENT '成团目标人数',
  `complete_count`  INT             NOT NULL DEFAULT 0 COMMENT '已支付人数',
  `lock_count`      INT             NOT NULL DEFAULT 0 COMMENT '已锁定人数（含未支付）',
  `status`          TINYINT         NOT NULL DEFAULT 0 COMMENT '0=拼团中 1=已成团 2=拼团失败 3=已成团含退款',
  `valid_start`     DATETIME        NOT NULL COMMENT '团有效期起始',
  `valid_end`       DATETIME        NOT NULL COMMENT '团有效期截止',
  `notify_type`     VARCHAR(8)      NOT NULL DEFAULT 'HTTP' COMMENT '回调类型 HTTP/MQ',
  `notify_url`      VARCHAR(512)    DEFAULT NULL COMMENT 'HTTP回调地址',
  `created_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_team_id` (`team_id`),
  KEY `idx_activity_status` (`activity_id`, `status`),
  KEY `idx_valid_end_status` (`valid_end`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='拼团队伍';

-- ============================================================
-- 6. orders — 用户订单
-- ============================================================
CREATE TABLE `orders` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `user_id`         VARCHAR(64)     NOT NULL COMMENT '用户ID',
  `team_id`         VARCHAR(16)     NOT NULL COMMENT '团ID -> teams.team_id',
  `order_id`        VARCHAR(16)     NOT NULL COMMENT '内部订单号（12位随机数，系统生成）',
  `activity_id`     BIGINT          NOT NULL COMMENT '活动ID -> activities.activity_id',
  `goods_id`        VARCHAR(32)     NOT NULL COMMENT '商品ID -> products.goods_id',
  `source`          VARCHAR(16)     NOT NULL COMMENT '来源（下单快照）',
  `channel`         VARCHAR(16)     NOT NULL COMMENT '渠道（下单快照）',
  `original_price`  DECIMAL(10,2)   NOT NULL COMMENT '商品原价（快照）',
  `deduction_price` DECIMAL(10,2)   NOT NULL COMMENT '折扣金额（快照）',
  `pay_price`       DECIMAL(10,2)   NOT NULL COMMENT '实付金额（快照）',
  `status`          TINYINT         NOT NULL DEFAULT 0 COMMENT '0=锁定(待支付) 1=已支付 2=已退款',
  `out_trade_no`    VARCHAR(64)     NOT NULL COMMENT '外部交易单号（调用方生成，幂等+全链路关联）',
  `out_trade_time`  DATETIME        DEFAULT NULL COMMENT '外部交易时间',
  `created_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_order_id` (`order_id`),
  UNIQUE KEY `uk_out_trade_no` (`out_trade_no`),
  KEY `idx_user_activity` (`user_id`, `activity_id`),
  KEY `idx_team_id` (`team_id`),
  KEY `idx_status_created` (`status`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户订单';

-- ============================================================
-- 7. notify_tasks — 回调通知任务
-- ============================================================
CREATE TABLE `notify_tasks` (
  `id`              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `activity_id`     BIGINT          NOT NULL COMMENT '活动ID',
  `team_id`         VARCHAR(16)     NOT NULL COMMENT '团ID',
  `category`        VARCHAR(64)     DEFAULT NULL COMMENT '回调分类：trade_settlement/trade_unpaid_refund/trade_paid_refund/trade_paid_team_refund',
  `notify_type`     VARCHAR(8)      NOT NULL DEFAULT 'HTTP' COMMENT 'HTTP / MQ',
  `notify_target`   VARCHAR(512)    DEFAULT NULL COMMENT 'HTTP回调URL 或 MQ topic',
  `retry_count`     INT             NOT NULL DEFAULT 0 COMMENT '已重试次数',
  `status`          TINYINT         NOT NULL DEFAULT 0 COMMENT '0=待发送 1=成功 2=重试中 3=失败(达上限)',
  `payload`         TEXT            NOT NULL COMMENT '回调参数 JSON',
  `uuid`            VARCHAR(128)    NOT NULL COMMENT '幂等去重 UUID',
  `created_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`      DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_uuid` (`uuid`),
  KEY `idx_status_created` (`status`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='回调通知任务';

-- ============================================================
-- 8. payments — 支付单
-- ============================================================
CREATE TABLE `payments` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `order_id`      VARCHAR(16)     NOT NULL COMMENT '内部订单号 -> orders.order_id（发给支付宝的out_trade_no）',
  `out_trade_no`  VARCHAR(64)     NOT NULL COMMENT '外部交易单号 -> orders.out_trade_no（冗余方便查询）',
  `user_id`       VARCHAR(64)     NOT NULL COMMENT '用户ID',
  `team_id`       VARCHAR(16)     NOT NULL COMMENT '团ID',
  `amount`        DECIMAL(10,2)   NOT NULL COMMENT '支付金额（= orders.pay_price）',
  `subject`       VARCHAR(256)    NOT NULL COMMENT '商品标题（展示在支付宝页面）',
  `trade_no`      VARCHAR(64)     DEFAULT NULL COMMENT '支付宝交易号（回调时回填）',
  `status`        TINYINT         NOT NULL DEFAULT 0 COMMENT '0=待支付 1=已支付 2=已关闭',
  `qr_code_url`   VARCHAR(512)    DEFAULT NULL COMMENT '扫码支付二维码链接',
  `pay_url`       VARCHAR(512)    DEFAULT NULL COMMENT 'H5/PC 支付跳转链接',
  `paid_at`       DATETIME        DEFAULT NULL COMMENT '支付完成时间',
  `expire_at`     DATETIME        NOT NULL COMMENT '支付超时时间',
  `created_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_order_id` (`order_id`),
  KEY `idx_out_trade_no` (`out_trade_no`),
  KEY `idx_status_expire` (`status`, `expire_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='支付单';

-- ============================================================
-- 9. payment_logs — 支付回调日志
-- ============================================================
CREATE TABLE `payment_logs` (
  `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `order_id`      VARCHAR(16)     NOT NULL COMMENT '关联订单 -> orders.order_id',
  `notify_id`     VARCHAR(128)    NOT NULL COMMENT '支付宝通知ID（notify_id，去重用）',
  `notify_raw`    TEXT            NOT NULL COMMENT '支付宝回调原始 JSON',
  `status`        TINYINT         NOT NULL DEFAULT 0 COMMENT '0=未验证 1=验签通过 2=验签失败',
  `trade_status`  VARCHAR(32)     DEFAULT NULL COMMENT '支付宝交易状态：WAIT_BUYER_PAY/TRADE_SUCCESS/TRADE_CLOSED',
  `verified_at`   DATETIME        DEFAULT NULL COMMENT '验签时间',
  `created_at`    DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notify_id` (`notify_id`),
  KEY `idx_order_id` (`order_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='支付回调日志';

-- ============================================================
-- 10. crowd_tags — 人群标签
-- ============================================================
CREATE TABLE `crowd_tags` (
  `id`          INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `tag_id`      VARCHAR(32)  NOT NULL COMMENT '人群标签ID',
  `tag_name`    VARCHAR(64)  NOT NULL COMMENT '标签名称',
  `tag_desc`    VARCHAR(256) NOT NULL DEFAULT '' COMMENT '标签描述',
  `statistics`  INT          NOT NULL DEFAULT 0 COMMENT '人群统计量',
  `created_at`  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_tag_id` (`tag_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='人群标签';

-- ============================================================
-- 11. crowd_tag_details — 人群标签明细
-- ============================================================
CREATE TABLE `crowd_tag_details` (
  `id`          INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `tag_id`      VARCHAR(32)  NOT NULL COMMENT '标签ID -> crowd_tags.tag_id',
  `user_id`     VARCHAR(64)  NOT NULL COMMENT '用户ID',
  `created_at`  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_tag_user` (`tag_id`, `user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='人群标签明细';

-- ============================================================
-- 12. crowd_tag_jobs — 人群标签任务
-- ============================================================
CREATE TABLE `crowd_tag_jobs` (
  `id`              INT UNSIGNED NOT NULL AUTO_INCREMENT,
  `tag_id`          VARCHAR(32)  NOT NULL COMMENT '标签ID',
  `batch_id`        VARCHAR(16)  NOT NULL COMMENT '批次ID',
  `tag_type`        TINYINT      NOT NULL DEFAULT 1 COMMENT '标签类型：1=参与量 2=消费金额',
  `tag_rule`        VARCHAR(16)  NOT NULL COMMENT '标签规则（如 N次）',
  `stat_start_time` DATETIME     NOT NULL COMMENT '统计开始时间',
  `stat_end_time`   DATETIME     NOT NULL COMMENT '统计结束时间',
  `status`          TINYINT      NOT NULL DEFAULT 0 COMMENT '0=初始 1=执行中 2=重置 3=完成',
  `created_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_batch_id` (`batch_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='人群标签任务';

-- ============================================================
-- 13. dynamic_configs — 动态配置（Go 版新增，对标 TCC）
-- ============================================================
CREATE TABLE `dynamic_configs` (
  `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `config_key`   VARCHAR(128)    NOT NULL COMMENT '配置键',
  `config_value` TEXT            NOT NULL COMMENT 'JSON 值',
  `version`      INT UNSIGNED    NOT NULL DEFAULT 1 COMMENT '版本号',
  `updated_by`   VARCHAR(64)     NOT NULL DEFAULT '' COMMENT '更新人',
  `updated_at`   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  `created_at`   DATETIME        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_key` (`config_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='动态配置';

-- ============================================================
-- 测试数据
-- ============================================================

-- 插入测试折扣（直减20元）
INSERT INTO `discounts` (`discount_id`, `name`, `description`, `plan_type`, `expression`) VALUES
('D001', '直减20元', '新用户直减20元优惠', 'ZJ', '20');

-- 插入测试活动
INSERT INTO `activities` (`activity_id`, `name`, `discount_id`, `group_type`, `target_count`, `take_limit`, `valid_minutes`, `status`, `start_time`, `end_time`) VALUES
(100123, '测试拼团活动', 'D001', 0, 3, 5, 5, 1, '2025-01-01 00:00:00', '2029-12-31 23:59:59');

-- 插入测试商品
INSERT INTO `products` (`goods_id`, `goods_name`, `original_price`) VALUES
('GOODS001', '测试商品-手写MyBatis', 100.00);

-- 插入商品-活动映射
INSERT INTO `activity_products` (`source`, `channel`, `goods_id`, `activity_id`) VALUES
('APP', 'WECHAT', 'GOODS001', 100123);
