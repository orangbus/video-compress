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
          <!-- 开源地址：GitHub 图标 + 仓库名 -->
          <button class="source-link" title="GitHub 开源仓库" @click="openSource">
            <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 19c-5 1.5-5-2.5-7-3m14 6v-3.87a3.37 3.37 0 0 0-.94-2.61c3.14-.35 6.44-1.54 6.44-7A5.44 5.44 0 0 0 20 4.77 5.07 5.07 0 0 0 19.91 1S18.73.65 16 2.48a13.38 13.38 0 0 0-7 0C6.27.65 5.09 1 5.09 1A5.07 5.07 0 0 0 5 4.77a5.44 5.44 0 0 0-1.5 3.78c0 5.42 3.3 6.61 6.44 7A3.37 3.37 0 0 0 9 18.13V22"/></svg>
            video-compress
          </button>

          <!-- 窗口控制按钮 -->
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
      </aside>

      <main class="main">
        <StartPage v-if="store.page === 'start'" />
        <ConvertingPage v-else-if="store.page === 'converting'" />
        <DonePage v-else-if="store.page === 'done'" />
        <HelpPage v-else-if="store.page === 'help'" />
      </main>
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
</style>
