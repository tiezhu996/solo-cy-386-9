-- 001_init.sql 参考迁移脚本：GORM AutoMigrate 会生成等价结构。
-- users
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(32) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    nickname VARCHAR(32) NOT NULL,
    email VARCHAR(128),
    phone VARCHAR(20),
    avatar VARCHAR(512),
    role VARCHAR(16) NOT NULL DEFAULT 'user',
    credit_score INTEGER NOT NULL DEFAULT 100,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
-- products
CREATE TABLE IF NOT EXISTS products (
    id BIGSERIAL PRIMARY KEY,
    seller_id BIGINT NOT NULL,
    title VARCHAR(128) NOT NULL,
    description TEXT NOT NULL,
    original_price NUMERIC(12,2) NOT NULL,
    price NUMERIC(12,2) NOT NULL,
    condition VARCHAR(32) NOT NULL DEFAULT 'almost_new',
    category VARCHAR(32) NOT NULL DEFAULT 'other',
    images TEXT,
    status VARCHAR(32) NOT NULL DEFAULT 'on_sale',
    view_count INTEGER NOT NULL DEFAULT 0,
    favorite_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
-- favorites
CREATE TABLE IF NOT EXISTS favorites (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uk_fav_user_product UNIQUE (user_id, product_id)
);
-- addresses
CREATE TABLE IF NOT EXISTS addresses (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    receiver_name VARCHAR(32) NOT NULL,
    phone VARCHAR(20) NOT NULL,
    province VARCHAR(32) NOT NULL,
    city VARCHAR(32) NOT NULL,
    district VARCHAR(32),
    detail VARCHAR(255) NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- cart_items
CREATE TABLE IF NOT EXISTS cart_items (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 1,
    selected BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- orders
CREATE TABLE IF NOT EXISTS orders (
    id BIGSERIAL PRIMARY KEY,
    order_no VARCHAR(40) NOT NULL UNIQUE,
    buyer_id BIGINT NOT NULL,
    seller_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    address_id BIGINT NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 1,
    total_price NUMERIC(12,2) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending_payment',
    remark VARCHAR(255),
    paid_at TIMESTAMPTZ,
    shipped_at TIMESTAMPTZ,
    received_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    active_refund_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
-- 售后单：一个订单终身至多一条（order_id 唯一索引，杜绝重复申请）
CREATE TABLE IF NOT EXISTS refunds (
    id BIGSERIAL PRIMARY KEY,
    refund_no VARCHAR(40) NOT NULL UNIQUE,
    order_id BIGINT NOT NULL UNIQUE,
    buyer_id BIGINT NOT NULL,
    seller_id BIGINT NOT NULL,
    type VARCHAR(32) NOT NULL,
    reason VARCHAR(500) NOT NULL,
    apply_amount NUMERIC(12,2) NOT NULL,
    evidence TEXT,
    status VARCHAR(32) NOT NULL DEFAULT 'pending_seller',
    proposal_amount NUMERIC(12,2),
    proposal_reason VARCHAR(255) DEFAULT '',
    final_amount NUMERIC(12,2),
    refunded_at TIMESTAMPTZ,
    closed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_refunds_buyer ON refunds (buyer_id);
CREATE INDEX IF NOT EXISTS idx_refunds_seller ON refunds (seller_id);
CREATE INDEX IF NOT EXISTS idx_refunds_status ON refunds (status);
-- 售后协商历史：只追加（INSERT），完整记录申请/同意/拒绝/方案/接受/撤销
CREATE TABLE IF NOT EXISTS refund_negotiations (
    id BIGSERIAL PRIMARY KEY,
    refund_id BIGINT NOT NULL,
    order_id BIGINT NOT NULL,
    actor_id BIGINT NOT NULL,
    actor_role VARCHAR(16) NOT NULL,
    action VARCHAR(32) NOT NULL,
    amount NUMERIC(12,2) NOT NULL DEFAULT 0,
    remark VARCHAR(500),
    evidence TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_refund_negotiations_refund ON refund_negotiations (refund_id);
CREATE INDEX IF NOT EXISTS idx_refund_negotiations_order ON refund_negotiations (order_id);
-- 订单与进行中售后单关联（“售后中”标记，暂停发货/收货/完成/评价）
CREATE INDEX IF NOT EXISTS idx_orders_active_refund ON orders (active_refund_id);
-- messages
CREATE TABLE IF NOT EXISTS messages (
    id BIGSERIAL PRIMARY KEY,
    sender_id BIGINT NOT NULL,
    receiver_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    content VARCHAR(1000) NOT NULL,
    is_read BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- reviews
CREATE TABLE IF NOT EXISTS reviews (
    id BIGSERIAL PRIMARY KEY,
    order_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    reviewer_id BIGINT NOT NULL,
    reviewee_id BIGINT NOT NULL,
    rating VARCHAR(16) NOT NULL,
    content VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uk_review_order_reviewer UNIQUE (order_id, reviewer_id)
);
-- audit_logs
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT,
    username VARCHAR(32),
    module VARCHAR(32),
    action VARCHAR(64),
    resource_id VARCHAR(64),
    detail TEXT,
    ip VARCHAR(64),
    request_id VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
