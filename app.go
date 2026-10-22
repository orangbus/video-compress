package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"yasuo/internal/video"
)

// AppVersion 当前软件版本（与 GitHub Release tag 对应）
const AppVersion = "1.0.0"

// App 主应用结构
type App struct {
	ctx           context.Context
	mu            sync.Mutex
	env           video.EnvInfo
	envFresh      bool
	engine        *video.Engine
	forceQuitting bool // 用户已确认强制退出（放行 OnBeforeClose）
}

// UpdateInfo 版本检测结果
type UpdateInfo struct {
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	HasNew     bool   `json:"hasNew"`
	ReleaseURL string `json:"releaseUrl"`
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
			Desc:        "速度最快，体积约为原片 60%~70%，推荐日常批量使用；需 NVIDIA GTX 10 系以上显卡支持 HEVC",
			Tag:         "hevc_nvenc · QP 28 · p5 · 4 并行",
			Encoder:     "nvenc",
			CRF:         28,
			Preset:      "p5",
			Concurrency: 4,
		},
		{
			ID:          "quality",
			Title:       "更清晰的画质",
			Desc:        "质量数值调小，画质更清晰但文件更大，适合重要素材",
			Tag:         "hevc_nvenc · QP 26 · p5 · 4 并行",
			Encoder:     "nvenc",
			CRF:         26,
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

// OpenVideoFiles 弹出系统文件选择框（可多选），返回所选文件路径
func (a *App) OpenVideoFiles() []string {
	if a.ctx == nil {
		return nil
	}
	files, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择视频文件（可多选）",
		Filters: []runtime.FileFilter{
			{DisplayName: "视频文件", Pattern: "*.mp4;*.mkv;*.mov;*.avi;*.flv;*.wmv;*.ts;*.webm;*.m4v;*.rmvb;*.mpg;*.mpeg"},
			{DisplayName: "所有文件", Pattern: "*"},
		},
	})
	if err != nil {
		return nil
	}
	return files
}

// OpenVideoFolder 弹出系统文件夹选择框，返回所选目录（递归扫描）
func (a *App) OpenVideoFolder() string {
	if a.ctx == nil {
		return ""
	}
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择视频文件夹（递归扫描）",
	})
	if err != nil {
		return ""
	}
	return dir
}

// AddFiles 添加视频文件任务（重复添加自动去重）。outDir 非空时输出到该目录。
func (a *App) AddFiles(paths []string, outDir string) []*video.VideoTask {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e == nil {
		return nil
	}
	added := []*video.VideoTask{}
	for _, p := range paths {
		if t, err := e.AddPath(p, strings.TrimSpace(outDir)); err == nil {
			added = append(added, t)
		}
	}
	return added
}

// AddFolder 递归添加文件夹内的视频。outDir 非空时输出到该目录。
func (a *App) AddFolder(dir, outDir string) []*video.VideoTask {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e == nil {
		return nil
	}
	tasks, err := e.AddFolder(dir, strings.TrimSpace(outDir))
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
// ClearTasks 清空所有未在运行的任务（开始页"清空列表"用），返回移除数量
func (a *App) ClearTasks() int {
	a.mu.Lock()
	e := a.engine
	a.mu.Unlock()
	if e == nil {
		return 0
	}
	n := 0
	for _, t := range e.Snapshot() {
		if t.Status != video.StatusRunning {
			_ = e.RemoveTask(t.ID)
			n++
		}
	}
	return n
}

// ClearDone 清理完成页记录（含跨会话历史与状态文件），返回清理条数
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
	n += e.ClearHistory()
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
	// 开始前统一重算所有待处理任务的输出路径（含切换自定义/默认目录的场景），并按新路径重判跳过状态
	e.SetOutDir(opts.OutDir)
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

// --- 版本检测与关闭拦截 ---

// CheckUpdate 检查 GitHub 是否有新版本（网络失败时静默，返回当前版本）
func (a *App) CheckUpdate() UpdateInfo {
	info := UpdateInfo{
		Current:    AppVersion,
		Latest:     AppVersion,
		HasNew:     false,
		ReleaseURL: "https://github.com/orangbus/video-compress/releases",
	}
	client := &http.Client{Timeout: 6 * time.Second}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/orangbus/video-compress/releases/latest", nil)
	if err != nil {
		return info
	}
	req.Header.Set("User-Agent", "video-compress/"+AppVersion)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return info // 无法联网时不打扰，正常使用
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return info
	}
	var rel struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if json.NewDecoder(resp.Body).Decode(&rel) != nil {
		return info
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return info
	}
	info.Latest = rel.TagName
	if rel.HTMLURL != "" {
		info.ReleaseURL = rel.HTMLURL
	}
	info.HasNew = versionNewer(rel.TagName, AppVersion)
	return info
}

// BeforeClose 拦截窗口关闭：有正在压缩的任务时阻止关闭并通知前端弹自定义确认框
func (a *App) BeforeClose(ctx context.Context) bool {
	a.mu.Lock()
	e := a.engine
	force := a.forceQuitting
	a.mu.Unlock()
	// 用户已在确认弹窗里点了"退出程序"，直接放行
	if force {
		return false
	}
	if e == nil || !e.Running() {
		return false // 没有压缩任务，直接关闭
	}
	n := e.RunningCount()
	if ctx != nil {
		runtime.EventsEmit(ctx, "app:confirm-close", map[string]int{"count": n})
	}
	return true // 阻止关闭，等待用户确认
}

// ForceQuit 用户确认退出：停止压缩任务后彻底退出（不再触发关闭拦截）
func (a *App) ForceQuit() {
	a.mu.Lock()
	e := a.engine
	a.forceQuitting = true // 放行 BeforeClose，避免二次弹窗
	a.mu.Unlock()
	if e != nil {
		e.Stop()
	}
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

// versionNewer 判断 latest 是否比 current 新（支持 v 前缀，如 v0.1.0）
func versionNewer(latest, current string) bool {
	ls := strings.Split(normalizeVersion(latest), ".")
	cs := strings.Split(normalizeVersion(current), ".")
	for i := 0; i < len(ls) || i < len(cs); i++ {
		var x, y int
		if i < len(ls) {
			x, _ = strconv.Atoi(ls[i])
		}
		if i < len(cs) {
			y, _ = strconv.Atoi(cs[i])
		}
		if x > y {
			return true
		}
		if x < y {
			return false
		}
	}
	return false
}

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

// --- 其它 ---

// OpenFolder 在资源管理器中打开目录
// OpenFolder 在资源管理器中打开文件所在目录并选中该文件
func (a *App) OpenFolder(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err.Error()
	}
	// explorer /select,<文件> 会打开所在目录并选中文件；explorer 始终返回退出码 1，不能据此判断失败
	sel := abs
	selectFile := true
	if _, err := os.Stat(abs); err != nil {
		// 文件不存在（如已被删除）：回退到打开父目录
		dir := filepath.Dir(abs)
		if _, derr := os.Stat(dir); derr != nil {
			return "文件或目录不存在：" + abs
		}
		sel = dir
		selectFile = false
	}
	var cmd *exec.Cmd
	if selectFile {
		cmd = exec.Command("explorer.exe", "/select,"+sel)
	} else {
		cmd = exec.Command("explorer.exe", sel)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Start()
	return ""
}

// Ping 前端连接测试
func (a *App) Ping() string {
	return "pong"
}
