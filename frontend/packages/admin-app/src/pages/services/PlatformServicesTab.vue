<template>
  <div class="mb-4 flex justify-end">
    <a-button v-if="hasPermission('platform:create')" type="primary" @click="openCreate"> 新建平台服务 </a-button>
  </div>
  <a-table row-key="id" :columns="columns" :data="services" :loading="loading" :pagination="false">
    <template #name="{ record }">
      <div class="font-medium">{{ record.display_name }}</div>
      <div class="text-xs text-gray-500">{{ record.platform_key }} / {{ record.service_key }}</div>
    </template>
    <template #endpoint="{ record }">
      <div class="max-w-80 truncate" :title="record.control_endpoint">控制：{{ record.control_endpoint }}</div>
      <div class="max-w-80 truncate text-xs text-gray-500" :title="record.runtime_endpoint">
        运行：{{ record.runtime_endpoint }} · {{ record.runtime_server_name }}
      </div>
      <div class="text-xs text-gray-500">受众：{{ record.service_audience }}</div>
    </template>
    <template #state="{ record }">
      <a-space direction="vertical" size="mini">
        <a-tag :color="record.enabled ? 'green' : 'gray'">
          {{ record.enabled ? '已启用' : '已停用' }}
        </a-tag>
        <a-tag :color="runtimeColor(record.runtime_state)">{{ record.runtime_state }}</a-tag>
      </a-space>
    </template>
    <template #actions="{ record }">
      <a-space>
        <a-button
          v-if="hasPermission('platform:read')"
          type="text"
          size="small"
          :loading="checkingId === record.id"
          @click="check(record)"
        >
          检查
        </a-button>
        <a-button v-if="hasPermission('platform:update')" type="text" size="small" @click="openEdit(record)">
          编辑
        </a-button>
        <a-popconfirm
          v-if="hasPermission('platform:delete')"
          content="确认删除这个平台服务？仍被账号引用时后端会拒绝删除。"
          type="warning"
          @ok="remove(record)"
        >
          <a-button type="text" size="small" status="danger">删除</a-button>
        </a-popconfirm>
      </a-space>
    </template>
  </a-table>
  <a-empty v-if="!loading && services.length === 0" description="暂无平台服务" />

  <a-modal
    v-model:visible="modalVisible"
    :title="editingId ? '编辑平台服务' : '新建平台服务'"
    :mask-closable="false"
    :on-before-ok="save"
    @close="resetForm"
  >
    <a-form :model="form" layout="vertical">
      <a-form-item label="平台标识" required>
        <a-input v-model="form.platform_key" placeholder="mihomo" />
      </a-form-item>
      <a-form-item label="显示名称" required><a-input v-model="form.display_name" /></a-form-item>
      <a-form-item label="服务标识" required>
        <a-input v-model="form.service_key" placeholder="mihomo" />
      </a-form-item>
      <a-form-item label="票据受众" required>
        <a-input v-model="form.service_audience" placeholder="platform-mihomo-service" />
      </a-form-item>
      <a-form-item label="发现方式" required>
        <a-input v-model="form.discovery_type" placeholder="static" />
      </a-form-item>
      <a-form-item label="控制端点（mTLS）" required>
        <a-input v-model="form.control_endpoint" placeholder="platform-mihomo:9000" />
      </a-form-item>
      <a-form-item label="运行端点（TLS）" required>
        <a-input v-model="form.runtime_endpoint" placeholder="runtime.example.com:443" />
      </a-form-item>
      <a-form-item label="运行端点 SNI" required>
        <a-input v-model="form.runtime_server_name" placeholder="runtime.example.com" />
      </a-form-item>
      <a-form-item label="支持动作" required>
        <a-textarea v-model="form.actionsText" :auto-size="{ minRows: 3, maxRows: 7 }" placeholder="每行一个动作" />
      </a-form-item>
      <a-form-item label="凭据 JSON Schema" required>
        <a-textarea v-model="form.schemaText" :auto-size="{ minRows: 5, maxRows: 10 }" />
      </a-form-item>
      <a-form-item label="启用"><a-switch v-model="form.enabled" /></a-form-item>
    </a-form>
  </a-modal>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Message } from '@arco-design/web-vue'
import { useUserStore, type PlatformService, type PlatformServiceInput } from '@paigram/shared-components'
import { adminServicesApi } from '@/api'

const userStore = useUserStore()
const hasPermission = (permission: string): boolean => userStore.hasPermission(permission)
const columns = [
  { title: '服务', slotName: 'name', width: 220 },
  { title: '端点 / 受众', slotName: 'endpoint' },
  { title: '发现方式', dataIndex: 'discovery_type', width: 110 },
  { title: '状态', slotName: 'state', width: 130 },
  { title: '操作', slotName: 'actions', width: 190 },
]
const services = ref<PlatformService[]>([])
const loading = ref(false)
const checkingId = ref<number | null>(null)
const editingId = ref<number | null>(null)
const modalVisible = ref(false)
const form = reactive({
  platform_key: '',
  display_name: '',
  service_key: '',
  service_audience: '',
  discovery_type: 'static',
  control_endpoint: '',
  runtime_endpoint: '',
  runtime_server_name: '',
  enabled: true,
  actionsText: '',
  schemaText: '{}',
})

const splitLines = (value: string): string[] => [
  ...new Set(
    value
      .split(/[\n,]/)
      .map((item) => item.trim())
      .filter(Boolean)
  ),
]

const load = async (): Promise<void> => {
  loading.value = true
  try {
    services.value = (await adminServicesApi.listPlatformServices()).data ?? []
  } finally {
    loading.value = false
  }
}

const openCreate = (): void => {
  resetForm()
  modalVisible.value = true
}

const openEdit = (service: PlatformService): void => {
  editingId.value = service.id
  Object.assign(form, {
    platform_key: service.platform_key,
    display_name: service.display_name,
    service_key: service.service_key,
    service_audience: service.service_audience,
    discovery_type: service.discovery_type,
    control_endpoint: service.control_endpoint,
    runtime_endpoint: service.runtime_endpoint,
    runtime_server_name: service.runtime_server_name,
    enabled: service.enabled,
    actionsText: (service.supported_actions ?? []).join('\n'),
    schemaText: JSON.stringify(service.credential_schema, null, 2),
  })
  modalVisible.value = true
}

const toInput = (): PlatformServiceInput | null => {
  const required = [
    form.platform_key,
    form.display_name,
    form.service_key,
    form.service_audience,
    form.discovery_type,
    form.control_endpoint,
    form.runtime_endpoint,
    form.runtime_server_name,
  ]
  if (required.some((value) => !value.trim())) {
    Message.warning('请填写所有必填字段')
    return null
  }
  try {
    const schema: unknown = JSON.parse(form.schemaText)
    if (!schema || Array.isArray(schema) || typeof schema !== 'object') {
      throw new Error('invalid schema')
    }
    return {
      platform_key: form.platform_key.trim(),
      display_name: form.display_name.trim(),
      service_key: form.service_key.trim(),
      service_audience: form.service_audience.trim(),
      discovery_type: form.discovery_type.trim(),
      control_endpoint: form.control_endpoint.trim(),
      runtime_endpoint: form.runtime_endpoint.trim(),
      runtime_server_name: form.runtime_server_name.trim(),
      enabled: form.enabled,
      supported_actions: splitLines(form.actionsText),
      credential_schema: schema as Record<string, unknown>,
    }
  } catch (_error) {
    Message.error('凭据 Schema 必须是有效的 JSON 对象')
    return null
  }
}

const save = async (): Promise<boolean> => {
  const input = toInput()
  if (!input) return false
  try {
    if (editingId.value) {
      await adminServicesApi.updatePlatformService(editingId.value, input)
      Message.success('平台服务已更新')
    } else {
      await adminServicesApi.createPlatformService(input)
      Message.success('平台服务已创建')
    }
    await load()
    return true
  } catch (_error) {
    return false
  }
}

const check = async (service: PlatformService): Promise<void> => {
  checkingId.value = service.id
  try {
    await adminServicesApi.checkPlatformService(service.id)
    Message.success('运行状态已更新')
    await load()
  } finally {
    checkingId.value = null
  }
}

const remove = async (service: PlatformService): Promise<void> => {
  await adminServicesApi.deletePlatformService(service.id)
  Message.success('平台服务已删除')
  await load()
}

const resetForm = (): void => {
  editingId.value = null
  Object.assign(form, {
    platform_key: '',
    display_name: '',
    service_key: '',
    service_audience: '',
    discovery_type: 'static',
    control_endpoint: '',
    runtime_endpoint: '',
    runtime_server_name: '',
    enabled: true,
    actionsText: '',
    schemaText: '{}',
  })
}

const runtimeColor = (state: string): string =>
  ({ healthy: 'green', degraded: 'orange', unavailable: 'red' })[state] ?? 'gray'

onMounted(() => void load())
</script>
