<script setup>
import { computed } from 'vue'
import { store, showToast } from '../store'
import { Backend, fmtSize, fmtDuration, statusText } from '../bridge'

const rows = computed(() => store.history)

async function refresh() {
  store.history = await Backend.GetHistory()
}

async function openOut(task) {
  const err = await Backend.OpenFolder(task.outPath)
  if (err) showToast(err)
}

async function clearDone() {
  const n = await Backend.ClearDone()
  await refresh()
  showToast(`已清理 ${n} 条本次会话记录`)
}
</script>

<template>
  <div>
    <div class="row-between" style="margin-bottom: 16px">
      <div>
        <h2 class="page-title" style="margin-bottom: 4px">完成</h2>
        <p class="page-sub" style="margin: 0">已转化完成的文件（跨会话保留历史记录）</p>
      </div>
      <div style="display: flex; gap: 10px">
        <button class="btn" @click="refresh">刷新</button>
        <button class="btn btn-danger" v-if="rows.length" @click="clearDone">清理本次会话记录</button>
      </div>
    </div>

    <div class="table-wrap">
      <table>
        <thead>
          <tr>
            <th style="min-width: 200px">文件名</th>
            <th>状态</th>
            <th>转化时长</th>
            <th>压缩比</th>
            <th>大小</th>
            <th style="min-width: 200px">文件路径</th>
            <th style="min-width: 200px">保存位置</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="t in rows" :key="t.outPath">
            <td :title="t.absPath">
              {{ t.absPath.split(/[\\/]/).pop() }}
              <div v-if="t.note" class="task-note">{{ t.note }}</div>
            </td>
            <td><span class="status-pill" :class="t.status">{{ statusText[t.status] || t.status }}</span></td>
            <td>{{ fmtDuration(t.costSec) }}</td>
            <td>
              <template v-if="t.status === 'done'">
                <span :class="{ 'ratio-bad': t.ratio > 100 }">{{ t.ratio.toFixed(1) }}%</span>
                <span class="muted" style="font-size: 11px">
                  ({{ t.ratio <= 100 ? '省' : '增大' }} {{ Math.abs(100 - t.ratio).toFixed(1) }}%)
                </span>
              </template>
              <template v-else>-</template>
            </td>
            <td class="muted" style="font-size: 12px">
              <template v-if="t.status === 'done'">{{ fmtSize(t.inBytes) }} → {{ fmtSize(t.outBytes) }}</template>
              <template v-else>-</template>
            </td>
            <td class="path-cell" :title="t.absPath">{{ t.absPath }}</td>
            <td class="path-cell" :title="t.outPath">{{ t.outPath }}</td>
            <td>
              <button class="btn btn-ghost" style="padding: 4px 10px; font-size: 12px" @click="openOut(t)">打开位置</button>
            </td>
          </tr>
        </tbody>
      </table>
      <div v-if="!rows.length" class="empty">暂无完成记录，去「开始」页压缩第一批视频吧</div>
    </div>
  </div>
</template>
