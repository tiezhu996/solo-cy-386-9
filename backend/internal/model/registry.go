package model

// AllModels 返回需要持久化的全部模型（单一事实来源）。
//
// 生产 AutoMigrate 与测试的“业务数据安全检查”都必须从这里取模型清单，
// 避免在多处手写表名：新增实体时只需在此注册一次，
// 建表迁移与外部验证库的非空数据检查会同时覆盖它。
func AllModels() []interface{} {
	return []interface{}{
		&User{},
		&Product{},
		&Favorite{},
		&Address{},
		&CartItem{},
		&Order{},
		&Refund{},
		&RefundNegotiation{},
		&Message{},
		&Review{},
		&AuditLog{},
	}
}
