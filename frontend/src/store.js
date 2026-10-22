import { reactive } from 'vue'

// 全局响应式状态
export const store = reactive({
  page: 'start',             // 当前页面: start | converting | done | help
  env: null,                 // 环境信息
  envLoading: true,
  modes: [],                 // 推荐模式
  step: 1,                   // 开始页步骤: 1 选模式 | 2 选文件 | 3 并行数
  selectedMode: null,        // 当前选中的模式对象
  advancedOpen: false,       // 高级选项是否展开
  advanced: {
    enabled: false,
    crf: 18,
    preset: 'fast',
    encoder: 'libx265',
    concurrency: 4,
    outDir: '',
  },
  files: [],                 // 会话任务（列表页/转化页）
  history: [],               // 历史完成记录（完成页）
  converting: false,         // 是否正在压缩
  ffmpegDl: { active: false, percent: 0, downloaded: 0, total: 0 },
  toast: '',                 // 轻提示
  version: '0.1.0',          // 当前版本（与后端 AppVersion 保持一致）
  update: null,              // 新版本信息：{hasNew, latest, releaseUrl}，null=未检测到
  closeConfirm: null,        // 关闭确认：{count}，null=未弹出
})

let toastTimer = null
export function showToast(msg) {
  store.toast = msg
  if (toastTimer) clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { store.toast = '' }, 3500)
}

// 以模式为准刷新高级参数默认值
export function applyMode(mode) {
  store.selectedMode = mode
  store.advanced.crf = mode.crf
  store.advanced.preset = mode.preset
  store.advanced.encoder = mode.encoder
  store.advanced.concurrency = mode.concurrency
}

// 计算最终压缩参数（高级选项开启时用自定义值）
export function finalParams() {
  const adv = store.advanced
  if (adv.enabled) {
    return {
      crf: Number(adv.crf) || 18,
      preset: adv.preset,
      encoder: adv.encoder === 'nvenc' ? 'nvenc' : 'libx265',
      concurrency: Number(adv.concurrency) || 4,
      outDir: adv.outDir.trim(),
    }
  }
  const m = store.selectedMode
  return {
    crf: m ? m.crf : 18,
    preset: m ? m.preset : 'fast',
    encoder: m ? m.encoder : 'libx265',
    concurrency: m ? m.concurrency : 4,
    outDir: '',
  }
}

// 根据事件更新会话任务列表
export function upsertFile(task) {
  const idx = store.files.findIndex((f) => f.id === task.id)
  if (idx >= 0) store.files[idx] = task
  else store.files.push(task)
}

// 汇总统计
export function summarize() {
  let total = 0, done = 0, failed = 0, running = 0, skipped = 0
  for (const f of store.files) {
    total++
    if (f.status === 'done') done++
    else if (f.status === 'failed') failed++
    else if (f.status === 'running') running++
    else if (f.status === 'skipped') skipped++
  }
  return { total, done, failed, running, skipped }
}
