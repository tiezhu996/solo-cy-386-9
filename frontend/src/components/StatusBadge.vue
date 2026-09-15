<template>
  <el-tag :type="tagType" size="small" effect="light">{{ text }}</el-tag>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import {
  OrderStatusText,
  OrderStatusTag,
  ProductStatusText,
  ReviewRatingText,
  RefundStatusText,
  RefundStatusTag,
  RefundTypeText
} from '../constants'

const props = defineProps<{
  type: 'order' | 'product' | 'rating' | 'refund' | 'refundType'
  value: string
}>()

const text = computed(() => {
  if (props.type === 'order') return OrderStatusText[props.value] ?? props.value
  if (props.type === 'product') return ProductStatusText[props.value] ?? props.value
  if (props.type === 'rating') return ReviewRatingText[props.value] ?? props.value
  if (props.type === 'refundType') return RefundTypeText[props.value] ?? props.value
  return RefundStatusText[props.value] ?? props.value
})

const tagType = computed(() => {
  if (props.type === 'order') return (OrderStatusTag[props.value] ?? 'info') as any
  if (props.type === 'refund') return (RefundStatusTag[props.value] ?? 'info') as any
  if (props.type === 'refundType') return 'warning'
  if (props.type === 'product') return props.value === 'on_sale' ? 'success' : 'info'
  if (props.type === 'rating') {
    if (props.value === 'good') return 'success'
    if (props.value === 'bad') return 'danger'
    return 'warning'
  }
  return 'info'
})
</script>
