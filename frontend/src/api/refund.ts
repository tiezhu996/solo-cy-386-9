import request from '../utils/request'
import type { PageResult, RefundVO } from './types'

// 买家发起售后（一轮）：原因、金额、凭证；重复提交相同申请幂等返回原记录。
export function applyRefund(data: {
  order_id: number
  type: string
  reason: string
  amount: number
  evidence?: string[]
}) {
  return request.post('/refunds', data)
}

// 卖家同意售后。
export function agreeRefund(id: number) {
  return request.post(`/refunds/${id}/agree`)
}

// 卖家拒绝售后。
export function rejectRefund(id: number, reason: string) {
  return request.post(`/refunds/${id}/reject`, { reason })
}

// 卖家提出一次方案。
export function proposeRefund(id: number, data: { amount: number; reason: string }) {
  return request.post(`/refunds/${id}/propose`, data)
}

// 买家接受方案。
export function acceptRefund(id: number) {
  return request.post(`/refunds/${id}/accept`)
}

// 买家撤销售后（卖家处理前 / 方案待确认阶段）。
export function cancelRefund(id: number) {
  return request.post(`/refunds/${id}/cancel`)
}

export function getRefund(id: number) {
  return request.get(`/refunds/${id}`)
}

// 按订单回读售后（订单详情/列表回读一致）。
export function getRefundByOrder(orderId: number) {
  return request.get(`/orders/${orderId}/refund`)
}

export function listRefunds(params: Record<string, unknown>) {
  return request.get('/refunds', { params })
}

export type { RefundVO, PageResult }
