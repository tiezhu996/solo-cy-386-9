package handler

import (
	"github.com/marketpal/marketpal/internal/dto"
	"github.com/marketpal/marketpal/internal/model"
)

// orderVO 订单实体转视图对象（统一委托 dto.FromOrder，保证订单列表/详情/售后嵌套回读一致）。
func orderVO(o *model.Order) *dto.OrderVO {
	vo := dto.FromOrder(o)
	return &vo
}
