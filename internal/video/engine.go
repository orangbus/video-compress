package video

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// 任务状态
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
	StatusSkipped = "skipped" // 历史已完成且输出存在，本次自动跳过
)

// VideoTask 单个视频压缩任务
type VideoTask struct {
	ID       string  `json:"id"`       // 源文件绝对路径
	AbsPath  string  `json:"absPath"`  // 源文件绝对路径
	OutPath  string  `json:"outPath"`  // 输出文件绝对路径
	Size     int64   `json:"size"`     // 源文件字节
	Duration float64 `json:"duration"` // 视频时长（秒）
	Status   string  `json:"status"`   // pending/running/done/failed/skipped
	Progress float64 `json:"progress"` // 0-100
	Error    string  `json:"error"`
	StartAt  int64   `json:"startAt"` // unix ms
	EndAt    int64   `json:"endAt"`
	CostSec  float64 `json:"costSec"` // 转化耗时（秒）
	InBytes  int64   `json:"inBytes"`
	OutBytes int64   `json:"outBytes"`
	Ratio    float64 `json:"ratio"` // 输出/输入 %
	Skipped  bool    `json:"skipped"`
}

// Event 推送到前端的压缩事件
type Event struct {
	Type    string     `json:"type"` // started|task_update|task_done|task_failed|batch_done|stopped
	Task    *VideoTask `json:"task,omitempty"`
	Running int        `json:"running"`
	Done    int        `json:"done"`
	Failed  int        `json:"failed"`
	Total   int        `json:"total"`
	Stopped bool       `json:"stopped,omitempty"`
}

// Options 一次压缩的参数
type Options struct {
	Concurrency int
	CRF         int
	Preset      string
	Encoder     string
	OutDir      string // 可选；为空则输出到每个源文件目录下的 compressed
}

// Engine 压缩引擎：任务管理 + 并发 worker + 断点续传
type Engine struct {
	mu       sync.Mutex
	ffmpeg   string
	ffprobe  string
	emit     func(Event)
	tasks    map[string]*VideoTask // id -> 会话任务
	order    []string              // 任务添加顺序
	history  map[string]*VideoTask // outPath -> 已完成/失败记录（跨会话）
	stateDirs map[string]bool      // 本次会话接触过的状态目录
	running  bool
	cancel   context.CancelFunc
}

// NewEngine 创建引擎。emit 用于向前端推送事件（nil 则静默）。
func NewEngine(ffmpeg, ffprobe string, emit func(Event)) *Engine {
	e := &Engine{
		ffmpeg:    ffmpeg,
		ffprobe:   ffprobe,
		emit:      emit,
		tasks:     map[string]*VideoTask{},
		history:   map[string]*VideoTask{},
		stateDirs: map[string]bool{},
	}
	e.loadGlobalHistory()
	return e
}

// AddPath 添加一个视频文件任务。重复添加（按源路径）返回已有任务。
// outDir 为空时输出到 <源目录>/compressed。
func (e *Engine) AddPath(absPath, outDir string) (*VideoTask, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if t, ok := e.tasks[absPath]; ok {
		return t, nil
	}
	st, err := os.Stat(absPath)
	if err != nil {
		return nil, err
	}
	if !IsVideoFile(absPath) {
		return nil, fmt.Errorf("不支持的视频格式：%s", filepath.Ext(absPath))
	}

	outPath := computeOutPath(absPath, outDir)
	if dir := filepath.Dir(outPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	t := &VideoTask{
		ID:       absPath,
		AbsPath:  absPath,
		OutPath:  outPath,
		Size:     st.Size(),
		Status:   StatusPending,
		Progress: 0,
	}
	if e.ffprobe != "" {
		dur, w, h, fps := ProbeInfo(e.ffprobe, absPath)
		t.Duration = dur
		if w > 0 && h > 0 {
			t.AbsPath = absPath // 保持原值
			_ = w
			_ = h
			_ = fps
		}
	}

	// 断点续传：若历史记录中该文件已完成且输出存在，本次标记 skipped（自动跳过）
	if old, ok := e.history[outPath]; ok && old.Status == StatusDone {
		if _, err := os.Stat(outPath); err == nil {
			t.Status = StatusSkipped
			t.Skipped = true
			t.CostSec = old.CostSec
			t.InBytes = old.InBytes
			t.OutBytes = old.OutBytes
			t.Ratio = old.Ratio
			t.EndAt = old.EndAt
			t.Duration = old.Duration
		}
	}
	if t.Duration <= 0 {
		t.Duration = float64(ProbeDuration(e.ffprobe, absPath)) / 1e6
	}

	e.tasks[absPath] = t
	e.order = append(e.order, absPath)
	return t, nil
}

// AddFolder 递归扫描目录添加视频任务
func (e *Engine) AddFolder(dir, outDir string) ([]*VideoTask, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(absDir)
	if err != nil || !st.IsDir() {
		return nil, fmt.Errorf("目录不存在或不是目录：%s", absDir)
	}
	var added []*VideoTask
	err = filepath.WalkDir(absDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// 跳过输出目录，避免处理上一次的结果
			if outDir != "" {
				if samePath(p, outDir) {
					return filepath.SkipDir
				}
			} else if strings.EqualFold(filepath.Base(p), "compressed") {
				return filepath.SkipDir
			}
			return nil
		}
		if !IsVideoFile(p) {
			return nil
		}
		t, aerr := e.AddPath(p, outDir)
		if aerr == nil {
			added = append(added, t)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return added, nil
}

// RemoveTask 移除一个未在运行的任务
func (e *Engine) RemoveTask(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	t, ok := e.tasks[id]
	if !ok {
		return fmt.Errorf("任务不存在")
	}
	if t.Status == StatusRunning {
		return fmt.Errorf("任务正在压缩，无法移除")
	}
	delete(e.tasks, id)
	for i, v := range e.order {
		if v == id {
			e.order = append(e.order[:i], e.order[i+1:]...)
			break
		}
	}
	return nil
}

// RetryFailed 将失败的任务重新置为待压缩
func (e *Engine) RetryFailed() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, t := range e.tasks {
		if t.Status == StatusFailed {
			t.Status = StatusPending
			t.Error = ""
			t.Progress = 0
			t.Skipped = false
			n++
		}
	}
	return n
}

// Start 开始压缩队列中的 pending/failed 任务。异步执行，通过事件回调反馈进度。
func (e *Engine) Start(opts Options) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("已有压缩任务正在进行")
	}
	queue := []string{}
	for _, id := range e.order {
		t := e.tasks[id]
		if t.Status == StatusPending || t.Status == StatusFailed {
			t.Status = StatusPending
			t.Error = ""
			queue = append(queue, id)
		}
	}
	if len(queue) == 0 {
		e.mu.Unlock()
		return fmt.Errorf("没有待压缩的任务（可能全部已完成）")
	}
	e.running = true
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.mu.Unlock()

	go e.runBatch(ctx, queue, opts)
	return nil
}

// Stop 停止当前批次：终止进行中的 ffmpeg，剩余任务保持 pending
func (e *Engine) Stop() {
	e.mu.Lock()
	if e.cancel != nil {
		e.cancel()
	}
	e.mu.Unlock()
}

// UpdateBinaries 更新 ffmpeg/ffprobe 路径（如自动下载完成后再探测）
func (e *Engine) UpdateBinaries(ffmpeg, ffprobe string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ffmpeg = ffmpeg
	e.ffprobe = ffprobe
}

// Running 是否正在压缩
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// Snapshot 返回会话任务快照（按添加顺序）
func (e *Engine) Snapshot() []*VideoTask {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*VideoTask, 0, len(e.order))
	for _, id := range e.order {
		if t, ok := e.tasks[id]; ok {
			cp := *t
			out = append(out, &cp)
		}
	}
	return out
}

// History 返回历史完成/失败记录（跨会话），按完成时间倒序
func (e *Engine) History() []*VideoTask {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]*VideoTask, 0, len(e.history))
	for _, t := range e.history {
		if t.Status == StatusDone || t.Status == StatusFailed || t.Status == StatusSkipped {
			cp := *t
			out = append(out, &cp)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EndAt > out[j].EndAt })
	return out
}

// ---- 内部实现 ----

func (e *Engine) runBatch(ctx context.Context, queue []string, opts Options) {
	sem := make(chan struct{}, opts.Concurrency)
	var wg sync.WaitGroup

	total := len(queue)
	e.mu.Lock()
	e.emitIf(Event{Type: "started", Total: total, Done: e.countStatus(StatusDone, StatusSkipped), Failed: e.countStatus(StatusFailed)})
	e.mu.Unlock()

	for _, id := range queue {
		if ctx.Err() != nil {
			break
		}
		select {
		case sem <- struct{}{}:
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				defer func() { <-sem }()
				e.runOne(ctx, id, opts)
			}(id)
		case <-ctx.Done():
			goto done
		}
	}
done:
	wg.Wait()

	e.mu.Lock()
	e.running = false
	e.cancel = nil
	stopped := ctx.Err() != nil
	if stopped {
		// 停掉的运行中任务置为失败
		for _, id := range e.order {
			if t := e.tasks[id]; t != nil && t.Status == StatusRunning {
				t.Status = StatusFailed
				t.Error = "已手动停止"
				e.addHistoryLocked(t)
			}
		}
	}
	e.emitIf(Event{Type: "batch_done", Total: total, Done: e.countStatus(StatusDone, StatusSkipped), Failed: e.countStatus(StatusFailed), Stopped: stopped})
	e.mu.Unlock()
	e.persistGlobal()
}

func (e *Engine) runOne(ctx context.Context, id string, opts Options) {
	e.mu.Lock()
	t, ok := e.tasks[id]
	if !ok {
		e.mu.Unlock()
		return
	}
	t.Status = StatusRunning
	t.Progress = 0
	t.Error = ""
	t.StartAt = time.Now().UnixMilli()
	cp := *t
	e.mu.Unlock()
	e.emitIf(Event{Type: "task_update", Task: &cp, Total: len(e.order)})

	cmd := BuildCmd(Job{AbsPath: t.AbsPath, OutPath: t.OutPath}, opts.CRF, opts.Preset, opts.Encoder)
	durMicros := int64(t.Duration * 1e6)
	var lastPct float64

	outBytes, runErr := RunFFmpegProgress(ctx, e.ffmpeg, cmd, durMicros, func(delta int64) {
		if durMicros <= 0 {
			return
		}
		pct := float64(delta) / float64(durMicros) * 100
		e.mu.Lock()
		if t := e.tasks[id]; t != nil {
			if pct > 100 {
				pct = 100
			}
			// 按实际已编码进度累计
			base := 0.0
			_ = base
			if pct > t.Progress {
				t.Progress = pct
			}
			cp = *t
		}
		e.mu.Unlock()
		if pct-lastPct >= 0.5 || pct >= 100 {
			lastPct = pct
			e.emitIf(Event{Type: "task_update", Task: &cp, Total: len(e.order)})
		}
	})

	e.mu.Lock()
	t, ok = e.tasks[id]
	if !ok {
		e.mu.Unlock()
		return
	}
	// 手动停止：进度清零标记，避免显示为完成
	if ctx.Err() != nil && runErr != nil {
		t.Status = StatusFailed
		t.Error = "已手动停止"
		t.Progress = 0
	} else if runErr != nil {
		t.Status = StatusFailed
		t.Error = runErr.Error()
	} else {
		t.Status = StatusDone
		t.Progress = 100
		t.InBytes = t.Size
		t.OutBytes = outBytes
		if t.Size > 0 {
			t.Ratio = float64(outBytes) / float64(t.Size) * 100
		}
		t.CostSec = float64(time.Now().UnixMilli()-t.StartAt) / 1000
		t.EndAt = time.Now().UnixMilli()
		e.addHistoryLocked(t)
	}
	cp = *t
	evType := "task_done"
	if t.Status == StatusFailed {
		evType = "task_failed"
	}
	e.mu.Unlock()

	e.emitIf(Event{Type: evType, Task: &cp, Total: len(e.order), Done: e.countStatus(StatusDone, StatusSkipped), Failed: e.countStatus(StatusFailed)})
	e.persistDir(filepath.Dir(cp.OutPath))
}

// countStatus 统计状态数量（需持锁调用）
func (e *Engine) countStatus(states ...string) int {
	n := 0
	for _, t := range e.tasks {
		for _, s := range states {
			if t.Status == s {
				n++
			}
		}
	}
	return n
}

func (e *Engine) emitIf(ev Event) {
	if e.emit != nil {
		e.emit(ev)
	}
}

// addHistoryLocked 写入历史记录（需持锁）
func (e *Engine) addHistoryLocked(t *VideoTask) {
	e.history[t.OutPath] = t
}

// ---- 断点续传：状态文件 ----

// stateFileName 输出目录内的状态文件名
const stateFileName = ".yasuo_state.json"

// persistDir 把某输出目录下所有已完成/失败任务写入状态文件（断点续传用）
func (e *Engine) persistDir(outDir string) {
	e.mu.Lock()
	recs := []*VideoTask{}
	for _, t := range e.history {
		if strings.EqualFold(filepath.Dir(t.OutPath), outDir) {
			recs = append(recs, t)
		}
	}
	e.stateDirs[outDir] = true
	e.mu.Unlock()
	if len(recs) == 0 {
		return
	}
	writeJSON(filepath.Join(outDir, stateFileName), recs)
}

// persistGlobal 记录本次会话所有状态文件路径（跨会话恢复历史用）
func (e *Engine) persistGlobal() {
	e.mu.Lock()
	dirs := make([]string, 0, len(e.stateDirs))
	for d := range e.stateDirs {
		dirs = append(dirs, d)
	}
	e.mu.Unlock()
	writeJSON(filepath.Join(ExeDir(), ".yasuo_global_state.json"), dirs)
}

// loadGlobalHistory 启动时读取历史状态文件，恢复跨会话历史
func (e *Engine) loadGlobalHistory() {
	var dirs []string
	readJSON(filepath.Join(ExeDir(), ".yasuo_global_state.json"), &dirs)
	if len(dirs) == 0 {
		return
	}
	for _, d := range dirs {
		var recs []*VideoTask
		if readJSON(filepath.Join(d, stateFileName), &recs) == nil {
			for _, r := range recs {
				if r != nil && r.OutPath != "" {
					e.history[r.OutPath] = r
				}
			}
		}
	}
}

// computeOutPath 计算输出路径：outDir 为空 -> <源目录>/compressed/<文件名>
func computeOutPath(absPath, outDir string) string {
	base := filepath.Base(absPath)
	if outDir == "" {
		outDir = filepath.Join(filepath.Dir(absPath), "compressed")
	}
	return filepath.Join(outDir, base)
}

func samePath(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return strings.EqualFold(aa, bb)
}

func writeJSON(path string, v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	_ = os.WriteFile(path, data, 0o644)
}

func readJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
