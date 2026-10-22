package video

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// makeTestVideo 用 ffmpeg 生成一个 2 秒测试视频；ffmpeg 不可用时跳过
func makeTestVideo(t *testing.T, dir, name string) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	out := filepath.Join(dir, name)
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x180:rate=24",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac", out)
	if err := cmd.Run(); err != nil {
		t.Fatalf("make test video: %v", err)
	}
	return out
}

// waitIdle 等待引擎停止压缩
func waitIdle(t *testing.T, e *Engine, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for e.Running() {
		if time.Now().After(deadline) {
			t.Fatal("engine still running after timeout")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestEngineCompressAndResume(t *testing.T) {
	dir := t.TempDir()
	vid := makeTestVideo(t, dir, "a.mp4")

	emitCh := make(chan Event, 64)
	e := NewEngine("", "", func(ev Event) { emitCh <- ev })

	// 1) 添加并压缩
	task, err := e.AddPath(vid, "")
	if err != nil {
		t.Fatalf("AddPath: %v", err)
	}
	if task.Status != StatusPending {
		t.Fatalf("expected pending, got %s", task.Status)
	}
	if task.OutPath == "" || task.Duration <= 0 {
		t.Fatalf("outPath or duration not probed: %+v", task)
	}
	if err := e.Start(Options{Concurrency: 2, CRF: 18, Preset: "fast", Encoder: "libx265"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitIdle(t, e, 60*time.Second)

	task = e.Snapshot()[0]
	if task.Status != StatusDone {
		t.Fatalf("expected done, got %s (err=%s)", task.Status, task.Error)
	}
	if _, err := os.Stat(task.OutPath); err != nil {
		t.Fatalf("output missing: %v", err)
	}
	if task.Ratio <= 0 || task.CostSec <= 0 {
		t.Fatalf("ratio/cost not recorded: %+v", task)
	}

	// 2) 断点续传：再次添加同一文件 → 应标记 skipped
	e2 := NewEngine("", "", nil) // 模拟重启（读历史状态文件）
	task2, err := e2.AddPath(vid, "")
	if err != nil {
		t.Fatalf("AddPath 2: %v", err)
	}
	if task2.Status != StatusSkipped || !task2.Skipped {
		t.Fatalf("expected skipped on resume, got %s", task2.Status)
	}
	if err := e2.Start(Options{Concurrency: 1, CRF: 18, Preset: "fast", Encoder: "libx265"}); err == nil {
		t.Fatal("expected error when nothing to compress")
	}

	// 3) 历史记录可用
	hist := e2.History()
	if len(hist) == 0 || hist[0].Status != StatusDone {
		t.Fatalf("history not loaded: %+v", hist)
	}
}

func TestEngineFailAndRetry(t *testing.T) {
	dir := t.TempDir()
	// 伪造一个损坏的“视频”
	bad := filepath.Join(dir, "bad.mp4")
	if err := os.WriteFile(bad, []byte("not a video"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := NewEngine("", "", nil)
	if _, err := e.AddPath(bad, ""); err != nil {
		t.Fatalf("AddPath: %v", err)
	}
	if err := e.Start(Options{Concurrency: 1, CRF: 18, Preset: "fast", Encoder: "libx265"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitIdle(t, e, 30*time.Second)

	task := e.Snapshot()[0]
	if task.Status != StatusFailed {
		t.Fatalf("expected failed, got %s", task.Status)
	}
	if task.Error == "" {
		t.Fatal("expected error message")
	}

	// 重试后仍是待压缩状态（会再次失败，但状态流转正确）
	n := e.RetryFailed()
	if n != 1 {
		t.Fatalf("RetryFailed expected 1, got %d", n)
	}
	if e.Snapshot()[0].Status != StatusPending {
		t.Fatalf("expected pending after retry, got %s", e.Snapshot()[0].Status)
	}
}
