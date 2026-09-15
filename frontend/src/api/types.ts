// 全局类型定义。
export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

export interface PageResult<T> {
  list: T[]
  total: number
  page: number
  page_size: number
}

export interface UserVO {
  id: number
  username: string
  nickname: string
  email?: string
  phone?: string
  avatar?: string
  role: string
  credit_score: number
  status: string
  created_at: string
}

export interface ProductVO {
  id: number
  seller_id: number
  title: string
  description: string
  original_price: number
  price: number
  condition: string
  category: string
  images: string[]
  status: string
  view_count: number
  favorite_count: number
  created_at: string
  seller?: UserVO
  is_favorite?: boolean
}

export interface AddressVO {
  id: number
  receiver_name: string
  phone: string
  province: string
  city: string
  district?: string
  detail: string
  is_default: boolean
}

export interface OrderVO {
  id: number
  order_no: string
  buyer_id: number
  seller_id: number
  product_id: number
  address_id: number
  quantity: number
  total_price: number
  status: string
  remark?: string
  created_at: string
  active_refund_id?: number
  product?: ProductVO
  address?: AddressVO
  buyer?: UserVO
  seller?: UserVO
  active_refund?: RefundVO
  last_refund?: RefundVO
}

export interface RefundNegotiationVO {
  id: number
  actor_id: number
  actor_role: 'buyer' | 'seller'
  action: string
  amount: number
  remark?: string
  evidence?: string[]
  created_at: string
}

export interface RefundVO {
  id: number
  refund_no: string
  order_id: number
  order_no?: string
  buyer_id: number
  seller_id: number
  type: string
  reason: string
  apply_amount: number
  evidence?: string[]
  status: string
  proposal_amount?: number
  proposal_reason?: string
  final_amount?: number
  order_status?: string
  order_paid_amount?: number
  refunded_at?: string
  closed_at?: string
  created_at: string
  negotiations: RefundNegotiationVO[]
  order?: OrderVO
}

export interface ConversationVO {
  peer_id: number
  peer_name: string
  peer_avatar?: string
  product_id?: number
  product_name?: string
  last_content: string
  last_time: string
  unread_count: number
}

export interface MessageVO {
  id: number
  sender_id: number
  receiver_id: number
  product_id: number
  content: string
  is_read: boolean
  created_at: string
}

export interface ReviewVO {
  id: number
  order_id: number
  product_id: number
  reviewer_id: number
  reviewee_id: number
  rating: string
  content: string
  created_at: string
  reviewer?: UserVO
}

export interface CartItemVO {
  id: number
  product_id: number
  quantity: number
  selected: boolean
  product?: ProductVO
}

export interface AuditVO {
  id: number
  user_id: number
  username: string
  module: string
  action: string
  resource_id: string
  detail: string
  ip: string
  request_id: string
  created_at: string
}
