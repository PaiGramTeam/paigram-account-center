<template>
  <div class="space-y-3">
    <div ref="containerRef" class="min-h-[66px]"></div>
    <p v-if="errorMessage" class="text-sm text-red-500">{{ errorMessage }}</p>
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { getTurnstile, loadTurnstileScript } from '../../utils/turnstile'

interface Props {
  siteKey: string
  action: string
  theme?: 'light' | 'dark' | 'auto'
  size?: 'normal' | 'compact' | 'flexible'
  enabled?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  theme: 'auto',
  size: 'flexible',
  enabled: true,
})

const emit = defineEmits<{
  token: [token: string]
  error: [message: string]
  expired: []
}>()

const containerRef = ref<HTMLDivElement | null>(null)
const widgetId = ref<string | null>(null)
const errorMessage = ref('')

async function renderWidget() {
  if (!props.enabled || !props.siteKey || !containerRef.value) {
    return
  }

  try {
    await loadTurnstileScript()
    await nextTick()

    const turnstile = getTurnstile()

    if (!containerRef.value || !turnstile) {
      throw new Error('Turnstile is unavailable')
    }

    clearWidget()
    errorMessage.value = ''

    widgetId.value = turnstile.render(containerRef.value, {
      sitekey: props.siteKey,
      action: props.action,
      theme: props.theme,
      size: props.size,
      callback: (token: string) => {
        errorMessage.value = ''
        emit('token', token)
      },
      'expired-callback': () => {
        errorMessage.value = 'Verification expired, please complete the check again'
        emit('expired')
      },
      'error-callback': () => {
        errorMessage.value = 'Security check failed to load, please refresh and try again'
        emit('error', errorMessage.value)
      },
    })
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Security check failed to load'
    emit('error', errorMessage.value)
  }
}

function clearWidget() {
  if (containerRef.value) {
    containerRef.value.innerHTML = ''
  }

  const turnstile = getTurnstile()

  if (widgetId.value && turnstile) {
    try {
      turnstile.remove(widgetId.value)
    } catch {
      // ignore remove errors from stale widgets
    }
  }

  widgetId.value = null
}

function resetWidget() {
  const turnstile = getTurnstile()

  if (widgetId.value && turnstile) {
    turnstile.reset(widgetId.value)
  } else {
    void renderWidget()
  }
}

defineExpose({
  reset: resetWidget,
})

watch(
  () => [props.enabled, props.siteKey, props.action] as const,
  async ([enabled]) => {
    if (!enabled) {
      clearWidget()
      return
    }
    await renderWidget()
  }
)

onMounted(async () => {
  await renderWidget()
})

onBeforeUnmount(() => {
  clearWidget()
})
</script>
