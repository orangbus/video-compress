<script setup>
import { ref, computed } from 'vue'
import { store, showToast, applyMode, finalParams, addPending, removePending, resetBatch } from '../store'
import { Backend, fmtSize } from '../bridge'

const dragging = ref(false)
const startError = ref('')

// 环境状态文案
const envItems = computed(() => {
  const e = store.env
  if (!e) return []
  return [
    { k: 'CPU', v: e.cpuModel || '未知', ok: true },
    { k: 'CPU 核心', v: e.cpuCores + ' 逻辑核', ok: true },
    { k: '内存', v: e.memoryGB + ' GB', ok: true },
    { k: '显卡', v: e.gpuName || '未检测到', ok: true },
    { k: 'HEVC 硬编', v: e.nvencHevc ? '支持 (nvenc)' : '不支持', ok: e.nvencHevc },
    {
      k: 'FFmpeg',
      v: e.ffmpegVer ? e.ffmpegVer.split(' ').slice(0, 2).join(' ') : '未安装',
      ok: e.ready,
    },
  ]
})

async function downloadFFmpeg() {
  store.ffmpegDl.active = true
  store.ffmpegDl.percent = 0
  showToast('开始下载 ffmpeg（约 80MB），请稍候...')
  try {
    const binDir = await Backend.DownloadFFmpeg()
    store.ffmpegDl.active = false
    if (binDir) {
      store.env = await Backend.RefreshEnv()
      showToast('ffmpeg 安装成功：' + binDir)
    }
  } catch (e) {
    store.ffmpegDl.active = false
    showToast('下载失败：' + e)
  }
}

function selectMode(m) {
  applyMode(m)
}

// 步骤导航（两步制：1 选模式/参数 -> 2 添加视频并直接开始）
const canNext1 = computed(() => !!store.selectedMode)
const pendingFiles = computed(() =>
  store.pendingIds.map((id) => store.files.find((f) => f.id === id)).filter(Boolean)
)
const canStart = computed(() => pendingFiles.value.some((f) => f.status === 'pending' || f.status === 'failed' || f.status === 'skipped'))

function nextStep() {
  if (store.step === 1 && !canNext1.value) return
  store.step = 2
}

function prevStep() {
  if (store.step > 1) store.step--
}

// 当前高级选项里的输出目录（未设置时为空，输出到各源目录/compressed）
function currentOutDir() {
  return (store.advanced.outDir || '').trim()
}

// 文件选择（通过后端打开系统对话框）
async function pickFiles() {
  const res = await Backend.OpenVideoFiles()
  // 兼容返回单个字符串或字符串数组
  const paths = Array.isArray(res) ? res : res ? [res] : []
  if (paths.length) {
    await addPaths(paths)
  } else {
    showToast('未选择文件')
  }
}

async function pickFolder() {
  const dir = await Backend.OpenVideoFolder()
  if (dir) {
    const added = await Backend.AddFolder(dir, currentOutDir())
    if (added && added.length) {
      addPending(added)
      showToast(`已添加文件夹，共 ${added.length} 个视频`)
    } else {
      showToast('该文件夹内未找到支持的视频')
    }
  }
}

async function addPaths(paths) {
  const added = await Backend.AddFiles(paths, currentOutDir())
  if (added && added.length) {
    addPending(added)
    showToast(`已添加 ${added.length} 个视频`)
  } else {
    showToast('未找到支持的视频文件')
  }
}

function onDrop(e) {
  // WebView2 的 dataTransfer.files 拿不到真实路径，
  // 实际拖拽添加由 App.vue 全局 Runtime.OnFileDrop 处理（拿到系统路径）
  dragging.value = false
}

async function removeFile(id) {
  const err = await Backend.RemoveTask(id)
  if (err) {
    showToast(err)
    return
  }
  removePending(id)
}

// 清空当前批次（运行中的任务不能清）
async function clearFiles() {
  const n = await Backend.ClearTasks()
  store.pendingIds = []
  store.files = store.files.filter((f) => f.status === 'running')
  showToast(n > 0 ? `已清空 ${n} 个已添加任务` : '列表已清空')
}

async function startCompress() {
  startError.value = ''
  const p = finalParams()
  const err = await Backend.StartCompress(p.concurrency, p.crf, p.preset, p.encoder, p.outDir)
  if (err) {
    startError.value = err
    return
  }
  // 开始压缩后：向导重置回第一步，可重新选择模式和视频；后台任务继续在「转化」页显示
  resetBatch()
  store.converting = true
  store.page = 'converting'
}

// 汇总当前批次文件
const fileSummary = computed(() => {
  const n = pendingFiles.value.length
  const skipped = pendingFiles.value.filter((f) => f.status === 'done' || f.status === 'skipped').length
  return { n, skipped }
})

// 实际将使用的压缩参数标签（随高级选项实时变化）
const actualTag = computed(() => {
  const p = finalParams()
  const enc = p.encoder === 'nvenc' ? 'hevc_nvenc' : 'libx265'
  const q = p.encoder === 'nvenc' ? 'QP' : 'CRF'
  const out = p.outDir ? ` · 输出 ${p.outDir}` : ''
  return `${enc} · ${q} ${p.crf} · ${p.preset} · ${p.concurrency} 并行${out}`
})

// 参数过高（输出可能比原片大）实时警告
const qpWarn = computed(() => {
  const p = finalParams()
  if (p.encoder === 'nvenc' && p.crf < 26) {
    return `当前 QP ${p.crf} 接近无损，输出很可能比原片大。软件会自动重压直到不大于原片，但日常建议 QP 28 以上。`
  }
  if (p.encoder === 'libx265' && p.crf < 18) {
    return `当前 CRF ${p.crf} 接近无损，输出可能比原片大。建议 CRF 18~23。`
  }
  return ''
})
</script>

<template>
  <div>
    <h2 class="page-title">开始</h2>
    <p class="page-sub">检查环境 → 选择模式 → 添加视频 → 开始压缩</p>

    <!-- 步骤 1：环境 + 模式 -->
    <template v-if="store.step === 1">
      <!-- 环境检测卡片 -->
      <div class="card">
        <div class="card-title"><span class="dot"></span>电脑配置与 ffmpeg 环境检测</div>
        <div v-if="store.envLoading" class="muted">正在检测...</div>
        <div v-else-if="store.env" class="env-grid">
          <div v-for="(it, i) in envItems" :key="i" class="env-item" :class="it.ok ? 'ok' : 'bad'">
            <div class="k">{{ it.k }}</div>
            <div class="v" :title="it.v">{{ it.v }}</div>
          </div>
        </div>

        <!-- ffmpeg 缺失时 -->
        <div v-if="store.env && !store.env.ready" class="ffmpeg-actions">
          <button class="btn btn-primary" :disabled="store.ffmpegDl.active" @click="downloadFFmpeg">
            <span v-if="store.ffmpegDl.active" class="spin">⟳</span>
            {{ store.ffmpegDl.active ? '下载中...' : '自动下载 ffmpeg 到软件目录' }}
          </button>
          <span class="muted" style="font-size: 12px">下载约 80MB，安装后即可开始压缩</span>
          <div v-if="store.ffmpegDl.active" style="flex-basis: 100%">
            <div class="progress-track" style="max-width: 420px">
              <div class="progress-fill" :style="{ width: store.ffmpegDl.percent + '%' }"></div>
            </div>
            <div class="muted" style="font-size: 12px; margin-top: 4px">
              {{ store.ffmpegDl.percent }}%（{{ fmtSize(store.ffmpegDl.downloaded) }} / {{ fmtSize(store.ffmpegDl.total) }}）
            </div>
          </div>
        </div>
      </div>

      <!-- 模式选择 -->
      <div class="card">
        <div class="card-title"><span class="dot"></span>选择压缩模式（根据你的电脑推荐）</div>
        <div v-for="m in store.modes" :key="m.id" class="mode-card" :class="{ selected: store.selectedMode && store.selectedMode.id === m.id }" @click="selectMode(m)">
          <div class="mode-radio"></div>
          <div class="mode-body">
            <div class="t">
              {{ m.title }}
              <span v-if="m.recommended" class="badge-rec">推荐</span>
            </div>
            <div class="d">{{ m.desc }}</div>
          </div>
          <div class="mode-tag">{{ m.tag }}</div>
        </div>

        <!-- 高级选项 -->
        <div style="margin-top: 8px">
          <div class="adv-toggle" @click="store.advancedOpen = !store.advancedOpen">
            <span class="status-pill" :class="store.advanced.enabled ? 'done' : 'pending'" style="cursor: pointer" @click.stop="store.advanced.enabled = !store.advanced.enabled">
              {{ store.advanced.enabled ? '已启用' : '未启用' }}
            </span>
            <span>高级选项（自定义参数）</span>
            <span>{{ store.advancedOpen ? '▾' : '▸' }}</span>
          </div>

          <div v-if="store.advancedOpen" class="adv-grid">
            <div class="field">
              <label>编码器 encoder</label>
              <select v-model="store.advanced.encoder">
                <option value="libx265">libx265（CPU 软编，画质最好、文件最小）</option>
                <option value="nvenc">nvenc（GPU 硬编，速度最快，需 NVIDIA GTX10 系以上）</option>
              </select>
              <div class="hint">CPU 软编画质好、体积小但慢；GPU 硬编快 5~10 倍，日常批量推荐。</div>
            </div>
            <div class="field">
              <label>画质（GPU 为 QP / CPU 为 CRF，1~51，越小越清晰、文件越大）</label>
              <input v-model.number="store.advanced.crf" type="number" min="1" max="51" />
              <div class="hint">
                GPU 硬编：QP 28≈原片 70%（推荐）、QP 32≈50%；QP 小于 24 可能比原片还大。
                CPU 软编：CRF 18 画质很好、23 视觉几乎无差别（推荐）。
              </div>
            </div>
            <div class="field">
              <label>预设 preset（仅 CPU 软编生效，GPU 固定 p5）</label>
              <select v-model="store.advanced.preset">
                <option value="ultrafast">ultrafast（最快）</option>
                <option value="fast">fast（默认，平衡）</option>
                <option value="medium">medium</option>
                <option value="slow">slow（更慢、体积更小）</option>
                <option value="veryslow">veryslow（最慢、体积最小）</option>
              </select>
              <div class="hint">越慢压缩率越高。GPU 硬编自动使用 p5 预设，此项不生效。</div>
            </div>
            <div class="field">
              <label>并行数 concurrency（1~16）</label>
              <input v-model.number="store.advanced.concurrency" type="number" min="1" max="16" />
              <div class="hint">同时压缩几个文件。CPU 软编按核数推荐；GPU 硬编 4 个左右即可。</div>
            </div>
            <div class="field">
              <label>输出目录（留空 = 各源目录下的 compressed）</label>
              <input v-model="store.advanced.outDir" placeholder="例如 D:\压缩输出" />
              <div class="hint">所有视频输出到同一目录；留空则在每个视频所在文件夹下创建 compressed 子目录。</div>
            </div>
          </div>

          <!-- 实时参数回显：高级启用时显示实际将使用的参数 -->
          <div v-if="store.advanced.enabled" class="param-echo">
            <span class="status-pill done">将使用</span>
            <code>{{ actualTag }}</code>
          </div>
          <div v-if="store.advanced.enabled && qpWarn" class="param-warn">⚠ {{ qpWarn }}</div>
        </div>

        <div style="margin-top: 16px; text-align: right">
          <button class="btn btn-primary btn-lg" :disabled="!canNext1 || (store.env && !store.env.ready)" @click="nextStep">下一步</button>
          <div v-if="store.env && !store.env.ready" class="error-text">环境未就绪：请先安装 ffmpeg（可点击上方按钮自动下载）</div>
        </div>
      </div>
    </template>

    <!-- 步骤 2：已选模式 + 添加视频 + 直接开始 -->
    <template v-else-if="store.step === 2">
      <div class="stepper">
        <div class="step done"><span class="no">1</span>选择模式</div>
        <div class="step active"><span class="no">2</span>添加视频并开始</div>
      </div>

      <div class="card">
        <div class="card-title"><span class="dot"></span>已选择的模式</div>
        <div v-if="store.selectedMode" class="row-between">
          <div>
            <b>{{ store.selectedMode.title }}</b>
            <span class="muted" style="margin-left: 10px">{{ store.selectedMode.desc }}</span>
          </div>
          <!-- 高级已启用时显示实际参数，否则显示模式模板参数 -->
          <div class="mode-tag">{{ store.advanced.enabled ? actualTag : store.selectedMode.tag }}</div>
        </div>
      </div>

      <div class="card">
        <div class="card-title"><span class="dot"></span>添加视频（可批量选择 / 拖拽 / 选择整个文件夹）</div>
        <div class="dropzone" :class="{ drag: dragging }"
             @dragover.prevent="dragging = true"
             @dragleave.prevent="dragging = false"
             @drop.prevent="onDrop"
             @click="pickFiles">
          <div class="dz-title">拖拽视频到这里，或点击选择文件</div>
          <div class="dz-sub">支持 mp4 / mkv / mov / avi / flv / wmv / ts / webm 等格式</div>
        </div>

        <div style="display: flex; gap: 10px; margin-top: 12px; flex-wrap: wrap; align-items: center">
          <button class="btn" @click="pickFiles">选择视频文件</button>
          <button class="btn" @click="pickFolder">选择文件夹（递归）</button>
          <button class="btn btn-ghost" v-if="fileSummary.n" @click="clearFiles">清空列表</button>
          <span v-if="fileSummary.n" class="muted" style="font-size: 12px">
            已添加 {{ fileSummary.n }} 个视频{{ fileSummary.skipped ? `（${fileSummary.skipped} 个已有结果将跳过）` : '' }}
          </span>
        </div>

        <div v-if="pendingFiles.length" class="file-list">
          <div v-for="f in pendingFiles" :key="f.id" class="file-row">
            <span class="status-pill" :class="f.status">{{ f.status === 'done' ? '已完成' : f.status === 'skipped' ? '已跳过' : f.status === 'failed' ? '失败' : f.status === 'running' ? '压缩中' : '待压缩' }}</span>
            <span class="fname" :title="f.absPath">{{ f.absPath }}</span>
            <span class="fsize">{{ fmtSize(f.size) }}</span>
            <span class="fdel" title="移除" @click="removeFile(f.id)">✕</span>
          </div>
        </div>

        <div v-if="startError" class="error-text" style="margin-top: 10px">{{ startError }}</div>

        <div style="margin-top: 16px; display: flex; justify-content: space-between; align-items: center">
          <button class="btn" @click="prevStep">上一步</button>
          <button class="btn btn-primary btn-lg" :disabled="!canStart" @click="startCompress">开始压缩</button>
        </div>
        <div v-if="pendingFiles.length" class="muted" style="font-size: 12px; text-align: right; margin-top: 6px">
          已压缩过的文件会自动跳过；参数变化或旧结果比原片大时会自动重新压缩
        </div>
      </div>
    </template>
  </div>
</template>
