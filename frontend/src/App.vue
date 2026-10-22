<script setup>
import { ref, onMounted, onBeforeUnmount } from 'vue'
import { store, applyMode } from './store'
import { Backend, Runtime } from './bridge'
import StartPage from './pages/StartPage.vue'
import ConvertingPage from './pages/ConvertingPage.vue'
import DonePage from './pages/DonePage.vue'
import HelpPage from './pages/HelpPage.vue'

const svgIcons = {
  start: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/></svg>',
  converting: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>',
  done: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>',
  help: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>',
}

const navs = [
  { id: 'start', label: '开始', icon: svgIcons.start },
  { id: 'converting', label: '转化', icon: svgIcons.converting },
  { id: 'done', label: '完成', icon: svgIcons.done },
  { id: 'help', label: '说明', icon: svgIcons.help },
]

function switchPage(id) {
  store.page = id
}

// 无边框窗口控制（按钮在侧边栏底部）
const isMax = ref(false)

async function refreshMaxState() {
  try {
    isMax.value = await Runtime.WindowIsMaximised()
  } catch (e) {
    /* ignore */
  }
}

function winMin() { Runtime.WindowMinimise() }

function winMax() {
  Runtime.WindowToggleMaximise()
  setTimeout(refreshMaxState, 400)
}

function winClose() { Runtime.WindowClose() }

// 打开开源地址（图标 + 仓库名）
const SOURCE_URL = 'https://github.com/orangbus/video-compress'
function openSource() { Runtime.BrowserOpenURL(SOURCE_URL) }

// --- 新版本检测弹窗 ---
function onUpdateChecked(info) {
  if (info && info.hasNew) {
    store.version = info.current || store.version
    store.update = info
  }
}
function goDownload() {
  if (store.update) Runtime.BrowserOpenURL(store.update.releaseUrl)
}
function quitForUpdate() { Backend.ForceQuit() }

// --- 压缩中关闭确认弹窗 ---
function onConfirmClose(d) {
  store.closeConfirm = { count: (d && d.count) || 0 }
}
function keepCompressing() { store.closeConfirm = null }
function quitNow() { Backend.ForceQuit() }

// 事件订阅
function onVideoEvent(ev) {
  const t = ev.task
  if (t && (ev.type === 'task_update' || ev.type === 'task_done' || ev.type === 'task_failed')) {
    const idx = store.files.findIndex((f) => f.id === t.id)
    if (idx >= 0) store.files[idx] = t
  }
  if (ev.type === 'started') {
    store.converting = true
  }
  if (ev.type === 'batch_done') {
    store.converting = false
    refreshAll()
  }
  if (ev.type === 'error' && t && t.error) {
    store.toast = t.error
  }
}

async function refreshAll() {
  try {
    const [files, history, env] = await Promise.all([
      Backend.GetSessionTasks(),
      Backend.GetHistory(),
      Backend.GetEnv(),
    ])
    store.files = files || []
    store.history = history || []
    store.env = env
    store.envLoading = false
    store.converting = await Backend.CompressRunning()
  } catch (e) {
    console.error('refresh failed', e)
  }
}

onMounted(async () => {
  Runtime.On('video:event', onVideoEvent)
  Runtime.On('ffmpeg:progress', (p) => {
    store.ffmpegDl.active = true
    store.ffmpegDl.percent = p.percent || 0
    store.ffmpegDl.downloaded = p.downloaded || 0
    store.ffmpegDl.total = p.total || 0
  })
  Runtime.On('ffmpeg:done', () => {
    store.ffmpegDl.active = false
    store.ffmpegDl.percent = 100
  })
  Runtime.On('ffmpeg:error', (msg) => {
    store.ffmpegDl.active = false
    store.toast = 'ffmpeg 下载失败：' + msg
  })
  // 压缩中关闭窗口 -> 弹自定义确认框
  Runtime.On('app:confirm-close', onConfirmClose)
  await refreshAll()
  try {
    store.modes = await Backend.GetModes()
    if (!store.selectedMode && store.modes.length) {
      const rec = store.modes.find((m) => m.recommended) || store.modes[0]
      applyMode(rec)
    }
  } catch (e) {
    console.error(e)
  }
  // 启动时检查 GitHub 新版本（后台，不阻塞界面）
  try {
    const info = await Backend.CheckUpdate()
    onUpdateChecked(info)
  } catch (e) {
    console.error('update check failed', e)
  }
  // 监听窗口尺寸变化，切换最大化/还原图标
  window.addEventListener('resize', onResize)
  refreshMaxState()
})

function onResize() {
  setTimeout(refreshMaxState, 200)
}

onBeforeUnmount(() => {
  Runtime.Off('video:event', onVideoEvent)
  window.removeEventListener('resize', onResize)
})
</script>

<template>
  <div class="app-layout">
    <div class="app-body">
      <aside class="sidebar">
        <!-- 顶部 logo 区即窗口拖拽区（无边框窗口移动用） -->
        <div class="brand drag-region" title="按住此处可拖动窗口">
          <div class="logo">压</div>
          <div>
            <div class="name">视频压缩</div>
            <div class="author">作者：OrangBus</div>
          </div>
        </div>

        <nav>
          <button
            v-for="n in navs"
            :key="n.id"
            class="nav-item"
            :class="{ active: store.page === n.id }"
            @click="switchPage(n.id)"
          >
            <span v-html="n.icon"></span>
            {{ n.label }}
          </button>
        </nav>

        <div class="sidebar-status">
          <div v-if="store.env">ffmpeg：{{ store.env.ffmpegVer ? '已就绪' : '未安装' }}</div>
          <div v-if="store.converting" style="color: #7dd3fc">● 正在压缩...</div>
        </div>

        <div class="sidebar-footer">
          <!-- 开源地址：GitHub 图标 + 仓库名 + 版本号 -->
          <button class="source-link" title="GitHub 开源仓库" @click="openSource">
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"/></svg>
            <span>video-compress</span>
            <span class="ver">v{{ store.version }}</span>
          </button>
        </div>
      </aside>

      <main class="main">
        <StartPage v-if="store.page === 'start'" />
        <ConvertingPage v-else-if="store.page === 'converting'" />
        <DonePage v-else-if="store.page === 'done'" />
        <HelpPage v-else-if="store.page === 'help'" />
      </main>

      <!-- 窗口控制按钮（右上角） -->
      <div class="win-controls">
        <button class="wc-btn" title="最小化" @click="winMin">
          <svg viewBox="0 0 12 12" width="12" height="12"><line x1="2" y1="6" x2="10" y2="6" stroke="currentColor" stroke-width="1.4"/></svg>
        </button>
        <button class="wc-btn" :title="isMax ? '还原' : '最大化'" @click="winMax">
          <svg v-if="!isMax" viewBox="0 0 12 12" width="12" height="12"><rect x="1.5" y="1.5" width="9" height="9" rx="1" fill="none" stroke="currentColor" stroke-width="1.4"/></svg>
          <svg v-else viewBox="0 0 12 12" width="12" height="12"><path d="M1.5 4.5V1.5h8v8h-3" fill="none" stroke="currentColor" stroke-width="1.4"/><rect x="4" y="4" width="6.5" height="6.5" rx="1" fill="none" stroke="currentColor" stroke-width="1.4"/></svg>
        </button>
        <button class="wc-btn wc-close" title="关闭" @click="winClose">
          <svg viewBox="0 0 12 12" width="12" height="12"><line x1="2" y1="2" x2="10" y2="10" stroke="currentColor" stroke-width="1.4"/><line x1="10" y1="2" x2="2" y2="10" stroke="currentColor" stroke-width="1.4"/></svg>
        </button>
      </div>
    </div>

    <!-- 新版本提示弹窗（自绘） -->
    <div v-if="store.update" class="modal-mask" @click.self="quitForUpdate">
      <div class="modal-card">
        <div class="modal-title">
          <span class="modal-dot"></span>
          发现新版本
        </div>
        <div class="modal-body">
          <p>当前版本 <b>v{{ store.version }}</b>，GitHub 已发布 <b>v{{ store.update.latest }}</b>。</p>
          <p class="modal-warn">为保证功能可用，请先下载最新版再继续使用。</p>
        </div>
        <div class="modal-actions">
          <button class="btn-primary" @click="goDownload">去下载最新版</button>
          <button class="btn-plain" @click="quitForUpdate">退出程序</button>
        </div>
      </div>
    </div>

    <!-- 压缩中关闭确认弹窗（自绘） -->
    <div v-if="store.closeConfirm" class="modal-mask" @click.self="keepCompressing">
      <div class="modal-card">
        <div class="modal-title">
          <span class="modal-dot warn"></span>
          正在压缩中
        </div>
        <div class="modal-body">
          <p>还有 <b>{{ store.closeConfirm.count }}</b> 个视频正在压缩。</p>
          <p class="modal-warn">退出后当前任务将停止，进度会保留，下次打开可继续。</p>
        </div>
        <div class="modal-actions">
          <button class="btn-primary" @click="keepCompressing">继续压缩</button>
          <button class="btn-danger" @click="quitNow">退出程序</button>
        </div>
      </div>
    </div>

    <Transition name="fade">
      <div v-if="store.toast" class="toast">{{ store.toast }}</div>
    </Transition>
  </div>
</template>

<style scoped>
.toast {
  position: fixed;
  top: 20px;
  left: 50%;
  transform: translateX(-50%);
  background: #1f2937;
  color: #fff;
  padding: 10px 20px;
  border-radius: 10px;
  font-size: 13px;
  z-index: 100;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
  max-width: 70vw;
}
.fade-enter-active, .fade-leave-active { transition: opacity 0.25s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

/* ===== 自绘弹窗（新版本提示 / 关闭确认） ===== */
.modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.55);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 200;
  -webkit-app-region: no-drag;
  animation: mask-in 0.18s ease;
}
@keyframes mask-in { from { opacity: 0; } to { opacity: 1; } }

.modal-card {
  width: 420px;
  max-width: 86vw;
  background: #fff;
  border-radius: 16px;
  box-shadow: 0 24px 64px rgba(15, 23, 42, 0.28);
  padding: 22px 24px 18px;
  border-top: 3px solid #3b82f6;
  animation: card-in 0.2s ease;
}
@keyframes card-in { from { transform: translateY(10px); opacity: 0; } to { transform: none; opacity: 1; } }

.modal-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 700;
  color: #1e293b;
  margin-bottom: 14px;
}
.modal-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: linear-gradient(135deg, #60a5fa, #3b82f6);
}
.modal-dot.warn { background: linear-gradient(135deg, #fbbf24, #f59e0b); }

.modal-body { color: #475569; font-size: 13px; line-height: 1.75; }
.modal-body b { color: #1d4ed8; }
.modal-warn { color: #b45309; font-size: 12px; margin-top: 4px; }

.modal-actions {
  display: flex;
  gap: 10px;
  margin-top: 18px;
  justify-content: flex-end;
}
.btn-primary {
  border: none;
  padding: 8px 18px;
  border-radius: 9px;
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  background: linear-gradient(135deg, #4f8ef7, #2563eb);
  cursor: pointer;
  transition: filter 0.15s;
}
.btn-primary:hover { filter: brightness(1.08); }
.btn-plain {
  border: 1px solid #e2e8f0;
  padding: 8px 18px;
  border-radius: 9px;
  font-size: 13px;
  color: #475569;
  background: #f8fafc;
  cursor: pointer;
  transition: background 0.15s;
}
.btn-plain:hover { background: #f1f5f9; }
.btn-danger {
  border: none;
  padding: 8px 18px;
  border-radius: 9px;
  font-size: 13px;
  font-weight: 600;
  color: #fff;
  background: linear-gradient(135deg, #f87171, #dc2626);
  cursor: pointer;
  transition: filter 0.15s;
}
.btn-danger:hover { filter: brightness(1.08); }
</style>
