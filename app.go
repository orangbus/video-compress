package main

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"yasuo/internal/video"
)

// App 主应用结构
type App struct {
	ctx      context.Context
	mu       sync.Mutex
	env      video.EnvInfo
	envFresh bool
	engine   *video.Engine
}

// Mode 开始页的模式卡片
type Mode struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Desc         string `json:"desc"`
	Tag          string `json:"tag"` // 参数摘要
	Encoder      string `json:"encoder"`
	CRF          int    `json:"crf"`
	Preset       string `json:"preset"`
	Concurrency  int    `json:"concurrency"`
	Recommended  bool   `json:"recommended"`
}

// NewApp 创建 App
func NewApp() *App {
	return &App{}
}

// startup 在应用启动时保存上下文并初始化引擎
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	env := video.DetectEnv()
	a.mu.Lock()
	a.env = env
	a.envFresh = true
	a.engine = video.NewEngine(env.FFmpegPath, env.FFprobePath, a.pushEvent)
	a.mu.Unlock()
}

// pushEvent 把引擎事件推送到前端
func (a *App) pushEvent(ev video.Event) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "video:event", ev)
}

// --- 环境 ---

// GetEnv 返回电脑配置与 ffmpeg 环境信息（带缓存）
func (a *App) GetEnv() video.EnvInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.envFresh {
		return a.env
	}
	env := video.DetectEnv()
	a.env = env
	a.envFresh = true
	return env
}

// RefreshEnv 强制重新检测环境
func (a *App) RefreshEnv() video.EnvInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	env := video.DetectEnv()
	a.env = env
	a.envFresh = true
	if a.engine != nil {
		a.engine.UpdateBinaries(env.FFmpegPath, env.FFprobePath)
	}
	return env
}

// DownloadFFmpeg 自动下载 ffmpeg 到软件当前目录（阻塞至完成，进度通过 ffmpeg:progress 事件推送）
func (a *App) DownloadFFmpeg() string {
	binDir, err := video.DownloadFFmpeg(video.ExeDir(), func(downloaded, total int64, pct int) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "ffmpeg:progress", map[string]interface{}{
				"downloaded": downloaded,
				"total":      total,
				"percent":    pct,
			})
		}
	})
	if err != nil {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "ffmpeg:error", err.Error())
		}
		return ""
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "ffmpeg:done", binDir)
	}
	// 刷新环境缓存
	a.mu.Lock()
	env := video.DetectEnv()
	a.env = env
	a.envFresh = true
	if a.engine != nil {
		a.engine.UpdateBinaries(env.FFmpegPath, env.FFprobePath)
	}
	a.mu.Unlock()
	return binDir
}

// GetModes 返回 3 个推荐模式（按当前硬件）
func (a *App) GetModes() []Mode {
	env := a.GetEnv()
	cores := env.CPUCores
	if cores < 1 {
		cores = 4
	}
	cpuConc := cores / 2
	if cpuConc < 2 {
		cpuConc = 2
	}
	if cpuConc > 4 {
		cpuConc = 4
	}

	modes := []Mode{
		{
			ID:          "cpu",
			Title:       "CPU 软编",
			Desc:        "画质最好、文件最小，适合追求极限压缩比、不赶时间的场景",
			Tag:         fmt.Sprintf("libx265 · CRF 18 · fast · %d 并行", cpuConc),
			Encoder:     "libx265",
			CRF:         18,
			Preset:      "fast",
			Concurrency: cpuConc,
		},
		{
			ID:          "gpu",
			Title:       "GPU 硬编",
			Desc:        "速度最快，推荐日常批量使用；需 NVIDIA GTX 10 系以上显卡支持 HEVC",
			Tag:         "hevc_nvenc · CRF 18 · p5 · 4 并行",
			Encoder:     "nvenc",
			CRF:         18,
			Preset:      "p5",
			Concurrency: 4,
		},
		{
			ID:          "quality",
			Title:       "更清晰的画质",
			Desc:        "CRF 数值调小，画质更清晰但文件更大，适合重要素材",
			Tag:         "hevc_nvenc · CRF 16 · p5 · 4 并行",
			Encoder:     "nvenc",
			CRF:         16,
			Preset:      "p5",
			Concurrency: 4,
		},
	}
	// 有 NVIDIA + HEVC 硬编能力则推荐 GPU，否则推荐 CPU
	recommend := "cpu"
	if env.NVENCHEVC {
		recommend = "gpu"
	}
	for i := range modes {
		if modes[i].ID == recommend {
			modes[i].Recommended = true
		}
	}
	return modes
}

// --- 任务管理 ---

// AddFiles 添加视频文件任务（重复添加自动去重）
func (a *App) AddFiles(paths []string) []*video.VideoTask {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e == nil {
		return nil
	}
	added := []*video.VideoTask{}
	for _, p := range paths {
		if t, err := e.AddPath(p, ""); err == nil {
			added = append(added, t)
		}
	}
	return added
}

// AddFolder 递归添加文件夹内的视频
func (a *App) AddFolder(dir string) []*video.VideoTask {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e == nil {
		return nil
	}
	tasks, err := e.AddFolder(dir, "")
	if err != nil {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "video:event", video.Event{Type: "error", Task: &video.VideoTask{Error: err.Error()}})
		}
		return nil
	}
	return tasks
}

// RemoveTask 移除任务（运行中的无法移除）
func (a *App) RemoveTask(id string) string {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e == nil {
		return "引擎未初始化"
	}
	if err := e.RemoveTask(id); err != nil {
		return err.Error()
	}
	return ""
}

// RetryFailed 重试所有失败任务，返回重试数量
func (a *App) RetryFailed() int {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e == nil {
		return 0
	}
	return e.RetryFailed()
}

// ClearDone 清理已完成/跳过的任务（保留失败供重试）
func (a *App) ClearDone() int {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e == nil {
		return 0
	}
	n := 0
	for _, t := range e.Snapshot() {
		if t.Status == video.StatusDone || t.Status == video.StatusSkipped {
			_ = e.RemoveTask(t.ID)
			n++
		}
	}
	return n
}

// GetSessionTasks 返回本次会话的任务列表（含转换中/待压缩/失败）
func (a *App) GetSessionTasks() []*video.VideoTask {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e == nil {
		return nil
	}
	return e.Snapshot()
}

// GetHistory 返回历史完成/失败记录（跨会话，完成页用）
func (a *App) GetHistory() []*video.VideoTask {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e == nil {
		return nil
	}
	return e.History()
}

// --- 压缩控制 ---

// StartCompress 开始压缩。返回 "" 表示成功，否则返回错误信息。
// outDir 为空则输出到各源目录下的 compressed。
func (a *App) StartCompress(concurrency, crf int, preset, encoder, outDir string) string {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e == nil {
		return "引擎未初始化"
	}
	env := a.GetEnv()
	if !env.Ready {
		return "ffmpeg 环境未就绪，请先安装或自动下载 ffmpeg"
	}
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 16 {
		concurrency = 16
	}
	if crf < 1 {
		crf = 1
	}
	if crf > 51 {
		crf = 51
	}
	opts := video.Options{
		Concurrency: concurrency,
		CRF:         crf,
		Preset:      preset,
		Encoder:     encoder,
		OutDir:      strings.TrimSpace(outDir),
	}
	if err := e.Start(opts); err != nil {
		return err.Error()
	}
	return ""
}

// StopCompress 停止当前批次
func (a *App) StopCompress() {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e != nil {
		e.Stop()
	}
}

// CompressRunning 是否正在压缩
func (a *App) CompressRunning() bool {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	return e != nil && e.Running()
}

// --- 其它 ---

// OpenFolder 在资源管理器中打开目录
func (a *App) OpenFolder(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err.Error()
	}
	cmd := exec.Command("explorer", abs)
	if err := cmd.Start(); err != nil {
		return err.Error()
	}
	return ""
}

// Ping 前端连接测试
func (a *App) Ping() string {
	return "pong"
}
