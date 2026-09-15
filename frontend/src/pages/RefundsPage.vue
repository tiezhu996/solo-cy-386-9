<template>
  <el-card>
    <div class="head">
      <h2>售后管理</h2>
      <el-radio-group v-model="role" @change="load(1)">
        <el-radio-button value="buyer">我发起的</el-radio-button>
        <el-radio-button value="seller">我收到的</el-radio-button>
      </el-radio-group>
    </div>
    <el-tabs v-model="status" @tab-change="load(1)">
      <el-tab-pane label="全部" name="all" />
      <el-tab-pane label="待卖家处理" name="pending_seller" />
      <el-tab-pane label="待买家确认" name="proposal_pending" />
      <el-tab-pane label="退款成功" name="agreed" />
      <el-tab-pane label="已拒绝" name="rejected" />
      <el-tab-pane label="已撤销" name="cancelled" />
    </el-tabs>

    <div v-loading="loading">
      <el-card v-for="r in refunds" :key="r.id" class="refund-card" shadow="never">
        <div class="r-head">
          <div>
            <StatusBadge type="refundType" :value="r.type" />
            <span class="no">售后单号：{{ r.refund_no }}</span>
          </div>
          <StatusBadge type="refund" :value="r.status" />
        </div>
        <div class="r-body">
          <el-image v-if="r.order?.product?.images?.length" :src="r.order.product.images[0]" fit="cover" class="thumb" />
          <div class="info" @click="openDetail(r)">
            <div class="title">{{ r.order?.product?.title || `订单 #${r.order_id}` }}</div>
            <div class="sub">申请金额 ¥{{ formatPrice(r.apply_amount) }} · 原因：{{ r.reason }}</div>
            <div class="sub" v-if="r.proposal_amount">卖家方案 ¥{{ formatPrice(r.proposal_amount) }}：{{ r.proposal_reason }}</div>
            <div class="sub success" v-if="r.final_amount">退款结果：¥{{ formatPrice(r.final_amount) }}（{{ r.refunded_at }}）</div>
          </div>
          <div class="r-actions">
            <el-button size="small" @click="openDetail(r)">查看详情</el-button>
            <!-- 买家操作 -->
            <template v-if="role === 'buyer'">
              <el-button v-if="r.status === 'proposal_pending'" type="primary" size="small" @click="accept(r)">接受方案</el-button>
              <el-button v-if="pendingStatuses.includes(r.status)" size="small" @click="cancel(r)">撤销申请</el-button>
            </template>
            <!-- 卖家操作 -->
            <template v-else>
              <template v-if="r.status === 'pending_seller'">
                <el-button type="primary" size="small" @click="sellerAction('agree', r)">同意</el-button>
                <el-button v-if="r.type === 'partial_refund'" type="warning" size="small" @click="sellerAction('propose', r)">提方案</el-button>
                <el-button type="danger" size="small" @click="sellerAction('reject', r)">拒绝</el-button>
              </template>
            </template>
          </div>
        </div>
      </el-card>
      <EmptyState v-if="!loading && refunds.length === 0" description="暂无售后记录" />
    </div>

    <div class="pager">
      <el-pagination background layout="prev, pager, next, total" :total="total" :page-size="pageSize"
        :current-page="page" @current-change="load" />
    </div>
  </el-card>

  <!-- 售后详情抽屉 -->
  <el-drawer v-model="detailVisible" title="售后详情" size="520px">
    <div v-if="current" class="detail">
      <el-descriptions :column="1" border size="small">
        <el-descriptions-item label="售后单号">{{ current.refund_no }}</el-descriptions-item>
        <el-descriptions-item label="类型">
          <StatusBadge type="refundType" :value="current.type" />
        </el-descriptions-item>
        <el-descriptions-item label="状态">
          <StatusBadge type="refund" :value="current.status" />
        </el-descriptions-item>
        <el-descriptions-item label="关联订单">
          <el-link type="primary" @click="$router.push(`/orders`); detailVisible = false">{{ current.order_no || current.order?.order_no }}</el-link>
        </el-descriptions-item>
        <el-descriptions-item label="实付金额">¥{{ formatPrice(current.order_paid_amount || current.order?.total_price) }}</el-descriptions-item>
        <el-descriptions-item label="申请金额">¥{{ formatPrice(current.apply_amount) }}</el-descriptions-item>
        <el-descriptions-item label="申请原因">{{ current.reason }}</el-descriptions-item>
        <el-descriptions-item label="凭证">
          <div v-if="current.evidence?.length" class="evidence">
            <el-image v-for="(u, i) in current.evidence" :key="i" :src="u" fit="cover" class="ev-img"
              :preview-src-list="current.evidence" :initial-index="i" />
          </div>
          <span v-else>无</span>
        </el-descriptions-item>
        <el-descriptions-item v-if="current.proposal_amount" label="卖家方案">
          ¥{{ formatPrice(current.proposal_amount) }} {{ current.proposal_reason }}
        </el-descriptions-item>
        <el-descriptions-item v-if="current.final_amount" label="退款结果">
          <span class="success">¥{{ formatPrice(current.final_amount) }}</span>
        </el-descriptions-item>
      </el-descriptions>

      <h4 class="tl-title">协商历史</h4>
      <RefundTimeline :items="current.negotiations" />

      <div class="detail-actions">
        <template v-if="role === 'buyer'">
          <el-button v-if="current.status === 'proposal_pending'" type="primary" @click="accept(current)">接受方案</el-button>
          <el-button v-if="pendingStatuses.includes(current.status)" @click="cancel(current)">撤销申请</el-button>
        </template>
        <template v-else-if="current.status === 'pending_seller'">
          <el-button type="primary" @click="sellerAction('agree', current)">同意</el-button>
          <el-button v-if="current.type === 'partial_refund'" type="warning" @click="sellerAction('propose', current)">提方案</el-button>
          <el-button type="danger" @click="sellerAction('reject', current)">拒绝</el-button>
        </template>
      </div>
    </div>
  </el-drawer>

  <RefundSellerDialog v-model="sellerDialog" :mode="sellerMode" :refund="current" @handled="onHandled" />
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import * as refundApi from '../api/refund'
import type { RefundVO } from '../api/types'
import StatusBadge from '../components/StatusBadge.vue'
import EmptyState from '../components/EmptyState.vue'
import RefundTimeline from '../components/RefundTimeline.vue'
import RefundSellerDialog from '../components/RefundSellerDialog.vue'
import { formatPrice } from '../utils/format'

const route = useRoute()
const role = ref<'buyer' | 'seller'>((route.query.role as 'buyer' | 'seller') || 'buyer')
const status = ref('all')
const refunds = ref<RefundVO[]>([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = 10
const pendingStatuses = ['pending_seller', 'proposal_pending']

const detailVisible = ref(false)
const current = ref<RefundVO | null>(null)
const sellerDialog = ref(false)
const sellerMode = ref<'agree' | 'reject' | 'propose'>('agree')

onMounted(async () => {
  await load(1)
  // 从订单页跳转过来时自动打开售后详情。
  if (route.query.id) {
    const id = Number(route.query.id)
    const target = refunds.value.find((r) => r.id === id)
    if (target) await openDetail(target)
  }
})

async function load(p: number) {
  loading.value = true
  try {
    const params: Record<string, unknown> = { page: p, page_size: pageSize, role: role.value }
    if (status.value !== 'all') params.status = status.value
    const res: any = await refundApi.listRefunds(params)
    refunds.value = res.data.list || []
    total.value = Number(res.data.total || 0)
    page.value = p
  } finally {
    loading.value = false
  }
}

async function openDetail(r: RefundVO) {
  try {
    const res: any = await refundApi.getRefund(r.id)
    current.value = res.data
    detailVisible.value = true
  } catch {
    // 错误提示由拦截器处理
  }
}

async function accept(r: RefundVO) {
  try {
    await ElMessageBox.confirm(`确认接受卖家 ¥${formatPrice(r.proposal_amount)} 的退款方案？接受后立即退款。`, '接受方案', {
      type: 'warning'
    })
  } catch {
    return
  }
  await refundApi.acceptRefund(r.id)
  ElMessage.success('已接受方案，退款成功')
  await refresh()
}

async function cancel(r: RefundVO) {
  try {
    await ElMessageBox.confirm('撤销售后后订单恢复原状态，确定撤销吗？', '撤销售后', { type: 'warning' })
  } catch {
    return
  }
  await refundApi.cancelRefund(r.id)
  ElMessage.success('售后已撤销，订单恢复原状态')
  await refresh()
}

function sellerAction(mode: 'agree' | 'reject' | 'propose', r: RefundVO) {
  current.value = r
  sellerMode.value = mode
  sellerDialog.value = true
}

async function onHandled() {
  await refresh()
}

async function refresh() {
  await load(page.value)
  if (current.value && detailVisible.value) {
    const res: any = await refundApi.getRefund(current.value.id)
    current.value = res.data
  }
}
</script>

<style scoped>
.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
}
.refund-card {
  margin-bottom: 12px;
}
.r-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 8px;
  border-bottom: 1px dashed #ebeef5;
}
.no {
  color: #909399;
  font-size: 13px;
  margin-left: 8px;
}
.r-body {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 12px 0;
}
.thumb {
  width: 64px;
  height: 64px;
  border-radius: 6px;
}
.info {
  flex: 1;
  cursor: pointer;
}
.title {
  font-weight: 600;
}
.sub {
  color: #909399;
  font-size: 13px;
  margin-top: 4px;
}
.success {
  color: #67c23a;
}
.r-actions {
  display: flex;
  gap: 8px;
}
.pager {
  display: flex;
  justify-content: center;
  margin-top: 16px;
}
.detail {
  padding: 0 4px;
}
.evidence {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}
.ev-img {
  width: 64px;
  height: 64px;
  border-radius: 4px;
}
.tl-title {
  margin: 20px 0 12px;
}
.detail-actions {
  margin-top: 20px;
  display: flex;
  gap: 8px;
}
</style>
