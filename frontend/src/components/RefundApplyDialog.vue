<template>
  <el-dialog v-model="visible" title="申请售后" width="520px" @closed="onClosed">
    <el-form :model="form" label-width="92px">
      <el-form-item label="订单号">
        <span class="muted">{{ order?.order_no }}</span>
      </el-form-item>
      <el-form-item label="售后类型">
        <el-radio-group v-model="form.type">
          <el-radio value="return_refund">退货退款（全额）</el-radio>
          <el-radio value="partial_refund">部分退款（不退货）</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item label="退款金额">
        <el-input-number v-model="form.amount" :min="0.01" :max="maxAmount" :precision="2" :step="10"
          :disabled="form.type === 'return_refund'" />
        <span v-if="form.type === 'return_refund'" class="muted">退货退款须按实付 ¥{{ formatPrice(order?.total_price || 0) }} 全额</span>
        <span v-else class="muted">实付 ¥{{ formatPrice(order?.total_price || 0) }}，不可超过</span>
      </el-form-item>
      <el-form-item label="申请原因">
        <el-input v-model="form.reason" type="textarea" :rows="3" maxlength="500" show-word-limit
          placeholder="请描述问题，如：商品与描述不符、存在划痕等（至少 2 个字）" />
      </el-form-item>
      <el-form-item label="凭证图片">
        <ImageUploader v-model="form.evidence" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="submitting" @click="submit">提交申请</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import ImageUploader from './ImageUploader.vue'
import { applyRefund } from '../api/refund'
import type { OrderVO } from '../api/types'
import { formatPrice } from '../utils/format'

const props = defineProps<{ modelValue: boolean; order: OrderVO | null }>()
const emit = defineEmits<{
  (e: 'update:modelValue', v: boolean): void
  (e: 'applied'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})
const maxAmount = computed(() => Number((props.order?.total_price || 0).toFixed(2)))
const submitting = ref(false)
const form = ref({ type: 'partial_refund', amount: 0, reason: '', evidence: [] as string[] })

// 每次打开按当前订单实付初始化金额。
watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      form.value = { type: 'partial_refund', amount: maxAmount.value, reason: '', evidence: [] }
    }
  }
)

// 退货退款只能按实付全额。
watch(
  () => form.value.type,
  (t) => {
    if (t === 'return_refund') form.value.amount = maxAmount.value
  }
)

function onClosed() {
  form.value = { type: 'partial_refund', amount: maxAmount.value, reason: '', evidence: [] }
}

async function submit() {
  if (!props.order) return
  if (form.value.reason.trim().length < 2) {
    ElMessage.warning('请填写至少 2 个字的申请原因')
    return
  }
  const amount = Number(form.value.amount.toFixed(2))
  if (form.value.type === 'return_refund' && amount !== maxAmount.value) {
    ElMessage.error('退货退款必须按实付金额全额申请')
    return
  }
  if (!(amount > 0) || amount > maxAmount.value) {
    ElMessage.error(`退款金额必须在 0.01 ~ ${maxAmount.value} 之间`)
    return
  }
  submitting.value = true
  try {
    await applyRefund({
      order_id: props.order.id,
      type: form.value.type,
      reason: form.value.reason.trim(),
      amount,
      evidence: form.value.evidence
    })
    ElMessage.success('售后申请已提交，等待卖家处理')
    visible.value = false
    emit('applied')
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
