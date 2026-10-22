<script setup>
import { computed } from 'vue'
import { store, showToast, summarize } from '../store'
import { Backend, fmtSize } from '../bridge'

const sum = computed(() => summarize())

const activeTasks = computed(() =>
  store.files.filter((f) => f.status === 'running' || f.status === 'pending' || f.status === 'failed')
)

async function stop() {
  await Backend.StopCompress()
  store.converting = false
  showToast('已请求停止，正在终止 ffmpeg...')
}

async function retryFailed() {
  const n = await Backend.RetryFailed()
  if (n) {
    showToast(`已将 ${n} 个失败任务重新加入队列`)
    if (!store.converting) {
      const p = currentParams()
      const err = await Backend.StartCompress(p.concurrency, p.crf, p.preset, p.encoder, p.outDir)
      if (err) showToast(err)
      else store.converting = true
    }
  } else {
    showToast('没有失败的任务')
  }
}

function currentParams() {
  return {
    concurrency: store.advanced.concurrency || 4,
    crf: store.advanced.crf || 18,
    preset: store.advanced.preset || 'fast',
    encoder: store.advanced.encoder || 'libx265',
    outDir: store.advanced.outDir || '',
  }
}

async function openOut(task) {
  const err = await Backend.OpenFolder(task.outPath)
  if (err) showToast(err)
}
</script>

<template>
  <div>
    <h2 class="page-title">转化</h2>
    <p class="page-sub">正在压缩的文件与实时进度（出错不会中断，失败任务可重试）</p>

    <div class="summary-bar">
      <div class="s total"><b>{{ sum.total }}</b>总数</div>
      <div class="s ok"><b>{{ sum.done }}</b>完成</div>
      <div class="s"><b>{{ sum.running }}</b>进行中</div>
      <div class="s fail"><b>{{ sum.failed }}</b>失败</div>
      <div class="s"><b>{{ sum.skipped }}</b>跳过</div>
      <div style="flex: 1"></div>
      <button class="btn btn-danger" v-if="store.converting" @click="stop">停止压缩</button>
      <button class="btn" v-if="sum.failed > 0" @click="retryFailed">重试失败 ({{ sum.failed }})</button>
    </div>

    <div v-if="!store.files.length" class="card">
      <div class="empty">还没有任务，去「开始」页添加视频吧</div>
    </div>

    <div v-else>
      <!-- 运行中/待压缩/失败 列表 -->
      <div v-for="f in activeTasks" :key="f.id" class="task-row">
        <span class="status-pill" :class="f.status">
          {{ f.status === 'running' ? '压缩中' : f.status === 'failed' ? '失败' : '待压缩' }}
        </span>
        <div style="flex: 1; min-width: 0">
          <div style="display: flex; justify-content: space-between; gap: 10px; margin-bottom: 4px">
            <span style="font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap" :title="f.absPath">{{ f.absPath }}</span>
            <span class="muted" style="font-size: 12px; flex-shrink: 0">
              {{ f.status === 'running' ? f.progress.toFixed(1) + '%' : '' }}
            </span>
          </div>
          <div class="progress-track">
            <div class="progress-fill" :class="{ done: f.status === 'done', failed: f.status === 'failed' }"
                 :style="{ width: (f.status === 'failed' ? 100 : f.progress) + '%' }"></div>
          </div>
          <div v-if="f.status === 'failed'" class="error-text" style="margin-top: 4px">{{ f.error }}</div>
          <div v-if="f.note" class="task-note">⟳ {{ f.note }}</div>
        </div>
        <span class="fsize muted" style="font-size: 12px; flex-shrink: 0">{{ fmtSize(f.size) }}</span>
      </div>

      <!-- 已完成/已跳过 -->
      <div style="margin-top: 18px" class="muted" v-if="sum.done + sum.skipped > 0">
        已完成 {{ sum.done }} 个，已跳过 {{ sum.skipped }} 个（去「完成」页查看明细）
      </div>
    </div>
  </div>
</template>
