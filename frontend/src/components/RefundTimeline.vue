<template>
  <el-timeline v-if="items.length">
    <el-timeline-item
      v-for="n in items"
      :key="n.id"
      :timestamp="n.created_at"
      placement="top"
      :type="dotType(n.action)"
    >
      <div class="row">
        <el-tag size="small" :type="n.actor_role === 'buyer' ? 'primary' : 'warning'">
          {{ n.actor_role === 'buyer' ? '买家' : '卖家' }}
        </el-tag>
        <b class="action">{{ RefundActionText[n.action] || n.action }}</b>
        <span v-if="n.amount > 0" class="amount">¥{{ formatPrice(n.amount) }}</span>
      </div>
      <div v-if="n.remark" class="remark">{{ n.remark }}</div>
      <div v-if="n.evidence?.length" class="evidence">
        <el-image
          v-for="(url, i) in n.evidence"
          :key="i"
          :src="url"
          fit="cover"
          class="thumb"
          :preview-src-list="n.evidence"
          :initial-index="i"
        />
      </div>
    </el-timeline-item>
  </el-timeline>
  <EmptyState v-else description="暂无协商记录" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import EmptyState from './EmptyState.vue'
import { RefundActionText } from '../constants'
import type { RefundNegotiationVO } from '../api/types'
import { formatPrice } from '../utils/format'

const props = defineProps<{ items: RefundNegotiationVO[] }>()
const items = computed(() => props.items || [])

function dotType(action: string) {
  if (action === 'agree' || action === 'accept') return 'success'
  if (action === 'reject') return 'danger'
  if (action === 'cancel') return 'info'
  if (action === 'propose') return 'warning'
  return 'primary'
}
</script>

<style scoped>
.row {
  display: flex;
  align-items: center;
  gap: 8px;
}
.action {
  color: #303133;
}
.amount {
  color: #f56c6c;
  font-weight: 600;
}
.remark {
  color: #606266;
  font-size: 13px;
  margin-top: 4px;
}
.evidence {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
.thumb {
  width: 56px;
  height: 56px;
  border-radius: 4px;
}
</style>
