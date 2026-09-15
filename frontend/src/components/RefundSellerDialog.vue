<template>
  <el-dialog :title="title" width="460px" v-model="visible">
    <el-form label-width="80px">
      <el-form-item label="售后类型">
        <StatusBadge type="refundType" :value="refund?.type || ''" />
        <span class="muted">申请金额 ¥{{ formatPrice(refund?.apply_amount || 0) }}</span>
      </el-form-item>
      <template v-if="mode === 'propose'">
        <el-form-item label="退款金额">
          <el-input-number v-model="amount" :min="0.01" :max="maxAmount" :precision="2" :step="10" />
          <span class="muted">不超过实付 ¥{{ formatPrice(maxAmount) }}</span>
        </el-form-item>
        <el-form-item label="方案说明">
          <el-input v-model="reason" type="textarea" :rows="3" maxlength="255" show-word-limit
            placeholder="说明方案理由（至少 2 个字）" />
        </el-form-item>
      </template>
      <template v-else-if="mode === 'reject'">
        <el-form-item label="拒绝原因">
          <el-input v-model="reason" type="textarea" :rows="3" maxlength="255" show-word-limit
            placeholder="说明拒绝理由（至少 2 个字）" />
        </el-form-item>
      </template>
      <template v-else>
        <el-alert type="warning" :closable="false" show-icon
          :title="`确认同意按 ¥${formatPrice(refund?.apply_amount || 0)} 退款？同意后退款立即生效。`" />
      </template>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button :type="mode === 'reject' ? 'danger' : 'primary'" :loading="submitting" @click="submit">
        {{ mode === 'agree' ? '确认同意' : mode === 'reject' ? '确认拒绝' : '提交方案' }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import StatusBadge from './StatusBadge.vue'
import { agreeRefund, proposeRefund, rejectRefund } from '../api/refund'
import type { RefundVO } from '../api/types'
import { formatPrice } from '../utils/format'

const props = defineProps<{
  modelValue: boolean
  mode: 'agree' | 'reject' | 'propose'
  refund: RefundVO | null
}>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'handled'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})
const title = computed(() =>
  props.mode === 'agree' ? '同意售后' : props.mode === 'reject' ? '拒绝售后' : '提出退款方案'
)
// 金额上限优先取订单实付，回退申请金额。
const maxAmount = computed(() =>
  Number((props.refund?.order_paid_amount || props.refund?.order?.total_price || props.refund?.apply_amount || 0).toFixed(2))
)
const amount = ref(maxAmount.value)
const reason = ref('')
const submitting = ref(false)

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      amount.value = maxAmount.value
      reason.value = ''
    }
  }
)

async function submit() {
  if (!props.refund) return
  if (props.mode === 'propose') {
    if (!(amount.value > 0) || amount.value > maxAmount.value) {
      ElMessage.error(`方案金额必须在 0.01 ~ ${maxAmount.value} 之间`)
      return
    }
  }
  if (props.mode !== 'agree' && reason.value.trim().length < 2) {
    ElMessage.warning('请填写至少 2 个字的说明')
    return
  }
  submitting.value = true
  try {
    if (props.mode === 'agree') {
      await agreeRefund(props.refund.id)
      ElMessage.success('已同意退款')
    } else if (props.mode === 'reject') {
      await rejectRefund(props.refund.id, reason.value.trim())
      ElMessage.success('已拒绝售后，订单恢复原状态')
    } else {
      await proposeRefund(props.refund.id, { amount: Number(amount.value.toFixed(2)), reason: reason.value.trim() })
      ElMessage.success('方案已提交，等待买家确认')
    }
    visible.value = false
    emit('handled')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.muted {
  color: #909399;
  font-size: 12px;
  margin-left: 8px;
}
</style>
