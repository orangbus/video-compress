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
  pendingIds: [],            // 当前批次待压缩任务 id（开始页步骤2；开始压缩后清空）
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

// 以模式为准刷新高级参数默认值。
// 高级选项已启用时只跟随编码器（GPU→nvenc / CPU→libx265），
// 保留用户已调整的画质/并行数/输出目录等自定义参数。
export function applyMode(mode) {
  store.selectedMode = mode
  store.advanced.encoder = mode.encoder
  if (!store.advanced.enabled) {
    store.advanced.crf = mode.crf
    store.advanced.preset = mode.preset
    store.advanced.concurrency = mode.concurrency
  }
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

// 把新添加的任务并入会话快照和当前批次列表
export function addPending(tasks) {
  if (!Array.isArray(tasks)) return
  for (const t of tasks) {
    if (!t || !t.id) continue
    const idx = store.files.findIndex((f) => f.id === t.id)
    if (idx >= 0) store.files[idx] = t
    else store.files.push(t)
    if (!store.pendingIds.includes(t.id)) store.pendingIds.push(t.id)
  }
}

// 从当前批次和会话快照中移除一个任务
export function removePending(id) {
  store.pendingIds = store.pendingIds.filter((x) => x !== id)
  store.files = store.files.filter((f) => f.id !== id)
}

// 一批已开始压缩：重置向导到第一步，允许重新选择模式和视频（后台任务不受影响）
export function resetBatch() {
  store.pendingIds = []
  store.step = 1
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
