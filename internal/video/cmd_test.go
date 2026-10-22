package video

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func joinArgs(args []string) string { return strings.Join(args, " ") }

// 高级选项参数必须真实映射到 ffmpeg 命令行
func TestBuildCmd_NvencParams(t *testing.T) {
	cmd := BuildCmd(Job{AbsPath: "a.mp4", OutPath: "b.mp4"}, 32, "fast", "nvenc")
	s := joinArgs(cmd)
	for _, want := range []string{"-c:v hevc_nvenc", "-rc constqp", "-qp 32", "-preset p5", "-c:a copy", "-tag:v hvc1"} {
		if !strings.Contains(s, want) {
			t.Errorf("nvenc 命令缺少 %q\n实际: %s", want, s)
		}
	}
	if strings.Contains(s, "-crf") {
		t.Errorf("nvenc 不应包含 -crf: %s", s)
	}
}

func TestBuildCmd_X265Params(t *testing.T) {
	cmd := BuildCmd(Job{AbsPath: "a.mp4", OutPath: "b.mp4"}, 23, "slow", "libx265")
	s := joinArgs(cmd)
	for _, want := range []string{"-c:v libx265", "-crf 23", "-preset slow", "-c:a copy", "-tag:v hvc1"} {
		if !strings.Contains(s, want) {
			t.Errorf("libx265 命令缺少 %q\n实际: %s", want, s)
		}
	}
}

// 高级选项的 QP/CRF 值变化应反映到命令参数（画质参数生效依据）
func TestBuildCmd_CrfValueChange(t *testing.T) {
	cmd26 := BuildCmd(Job{AbsPath: "a.mp4", OutPath: "b.mp4"}, 26, "p5", "nvenc")
	cmd32 := BuildCmd(Job{AbsPath: "a.mp4", OutPath: "b.mp4"}, 32, "p5", "nvenc")
	if !strings.Contains(joinArgs(cmd26), "-qp 26") || !strings.Contains(joinArgs(cmd32), "-qp 32") {
		t.Errorf("QP 值变化未反映到命令: %s / %s", joinArgs(cmd26), joinArgs(cmd32))
	}
}

// 高级选项指定输出目录时，待处理任务的输出路径必须切换（输出目录生效依据）
func TestSetOutDir(t *testing.T) {
	e := NewEngine("ffmpeg", "ffprobe", nil)
	dir := t.TempDir()
	src := filepath.Join(dir, "a.mp4")
	if err := os.WriteFile(src, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	task, err := e.AddPath(src, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(task.OutPath, "compressed") {
		t.Errorf("默认输出应在 compressed 下: %s", task.OutPath)
	}
	outDir := filepath.Join(dir, "myout")
	if n := e.SetOutDir(outDir); n != 1 {
		t.Fatalf("SetOutDir 应影响 1 个任务，实际 %d", n)
	}
	if !strings.HasPrefix(task.OutPath, outDir) {
		t.Errorf("输出目录未生效: %s", task.OutPath)
	}
}

// 自动重压档位：QP16 应依次 20→24→28→32（最多 4 次重压），之后停止
func TestNextCrfForSize_Nvenc(t *testing.T) {
	opts := Options{Encoder: "nvenc", CRF: 16, Preset: "p5"}
	want := []int{20, 24, 28, 32}
	got := []int{}
	for attempt := 0; attempt < 4; attempt++ {
		n, ok := nextCrfForSize(opts, attempt)
		if !ok {
			t.Fatalf("attempt %d 应可重压", attempt)
		}
		got = append(got, n)
		opts.CRF = n
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("nvenc 档位序列 = %v, want %v", got, want)
	}
	// attempt=4 之后不再重压
	if _, ok := nextCrfForSize(opts, 4); ok {
		t.Error("attempt=4 应停止重压")
	}
}

// 自动重压档位：libx265 每档 +6，到达上限 40 停止
func TestNextCrfForSize_X265(t *testing.T) {
	opts := Options{Encoder: "libx265", CRF: 18, Preset: "fast"}
	n, ok := nextCrfForSize(opts, 0)
	if !ok || n != 24 {
		t.Errorf("libx265 首档 = %d,ok=%v, want 24", n, ok)
	}
	// 已达上限不再重压
	opts.CRF = 40
	if _, ok := nextCrfForSize(opts, 0); ok {
		t.Error("CRF=40 应停止重压")
	}
}

// 防回归：视频扩展名识别
func TestIsVideoFile(t *testing.T) {
	cases := map[string]bool{
		"a.mp4":  true,
		"b.MKV":  true,
		"c.ts":   true,
		"d.d.ts": false,
		"e.txt":  false,
		"f.webm": true,
		"g.rmvb": true,
		"h.m2ts": false,
		"i.mpeg": true,
		"j.m4v":  true,
	}
	for p, want := range cases {
		if got := IsVideoFile(p); got != want {
			t.Errorf("IsVideoFile(%q) = %v, want %v", p, got, want)
		}
	}
}
