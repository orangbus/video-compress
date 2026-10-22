// Wails 后端桥接：直接调用 window.go / window.runtime（不依赖生成的绑定文件）
const go = () => window.go?.main?.App
const rt = () => window.runtime

export const Backend = {
  GetEnv: () => go().GetEnv(),
  RefreshEnv: () => go().RefreshEnv(),
  DownloadFFmpeg: () => go().DownloadFFmpeg(),
  GetModes: () => go().GetModes(),
  AddFiles: (paths) => go().AddFiles(paths),
  AddFolder: (dir) => go().AddFolder(dir),
  RemoveTask: (id) => go().RemoveTask(id),
  RetryFailed: () => go().RetryFailed(),
  ClearDone: () => go().ClearDone(),
  GetSessionTasks: () => go().GetSessionTasks(),
  GetHistory: () => go().GetHistory(),
  StartCompress: (concurrency, crf, preset, encoder, outDir) =>
    go().StartCompress(concurrency, crf, preset, encoder, outDir),
  StopCompress: () => go().StopCompress(),
  CompressRunning: () => go().CompressRunning(),
  OpenFolder: (path) => go().OpenFolder(path),
}

export const Runtime = {
  On: (name, cb) => rt()?.EventsOn(name, cb),
  Off: (name, cb) => rt()?.EventsOff(name, cb),
  OpenFileDialog: (opts) => rt()?.OpenFileDialog(opts),
  OpenDirectoryDialog: (opts) => rt()?.OpenDirectoryDialog(opts),
  BrowserOpenURL: (url) => rt()?.BrowserOpenURL(url),
  WindowMinimise: () => rt()?.WindowMinimise(),
  WindowToggleMaximise: () => rt()?.WindowToggleMaximise(),
  WindowIsMaximised: () => rt()?.WindowIsMaximised(),
  WindowClose: () => rt()?.WindowClose(),
}

// 文件大小格式化
export function fmtSize(bytes) {
  if (!bytes || bytes <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return v.toFixed(i === 0 ? 0 : 1) + ' ' + units[i]
}

// 时长格式化（秒 -> mm:ss / h:mm:ss）
export function fmtDuration(sec) {
  if (!sec || sec <= 0) return '-'
  sec = Math.round(sec)
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  const pad = (n) => String(n).padStart(2, '0')
  return h > 0 ? `${h}:${pad(m)}:${pad(s)}` : `${pad(m)}:${pad(s)}`
}

// 状态中文名
export const statusText = {
  pending: '待压缩',
  running: '压缩中',
  done: '已完成',
  failed: '失败',
  skipped: '已跳过',
}
