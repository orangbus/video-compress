// Package video 封装视频压缩的核心能力：ffmpeg 命令构造、进度解析、环境检测、自动下载与任务引擎。
package video

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// DefaultExts 默认支持处理的视频扩展名
var DefaultExts = []string{".mp4", ".mkv", ".mov", ".avi", ".flv", ".wmv", ".ts", ".webm", ".m4v", ".rmvb", ".mpg", ".mpeg"}

// Job 一个待压缩文件的最小描述
type Job struct {
	AbsPath string // 源文件绝对路径
	OutPath string // 输出文件绝对路径
}

// BuildCmd 根据参数构造 ffmpeg 命令行参数列表
func BuildCmd(j Job, crf int, preset, encoder string) []string {
	args := []string{"-y", "-i", j.AbsPath}

	switch encoder {
	case "nvenc": // GPU 硬编，需 GTX10 系列以上 (HEVC)
		// nvenc 用 constqp + QP 控制体积（实测有效：QP24≈原大小、QP28≈70%、QP32≈50%）。
		// 不要用 vbr+cq：该 ffmpeg 上 VBR 无码率约束会失控（输出可达原片 2-4 倍）。
		// QP 值与 x265 CRF 不可直接换算，QP 越小画质越好、文件越大。
		args = append(args,
			"-c:v", "hevc_nvenc",
			"-rc", "constqp",
			"-qp", fmt.Sprintf("%d", crf),
			"-preset", "p5",
			"-c:a", "copy",
		)
	default: // libx265 CPU 软编
		args = append(args,
			"-c:v", "libx265",
			"-crf", fmt.Sprintf("%d", crf),
			"-preset", preset,
			"-x265-params", "log-level=error", // 关闭 x265 大量日志，仅保留进度
			"-c:a", "copy",
		)
	}

	ext := strings.ToLower(extOf(j.OutPath))
	if ext == ".mp4" || ext == ".mov" {
		args = append(args, "-tag:v", "hvc1") // 兼容 iPhone/QuickTime 等播放器
	}
	args = append(args,
		"-map", "0:v:0",
		"-map", "0:a?",
		"-progress", "pipe:1",
		"-stats_period", "1", // 进度上报周期 1 秒，避免频繁刷新
		"-nostats",
		j.OutPath,
	)
	return args
}

// RunFFmpegProgress 执行 ffmpeg，解析 -progress 输出实时回调进度，返回输出文件字节数。
// onProgress 回调参数为累计已编码时长（微秒，out_time_us）。ctx 非 nil 时，取消 ctx 会终止 ffmpeg 进程。
// ffmpegPath 可为空（表示使用 PATH 中的 ffmpeg）。
func RunFFmpegProgress(ctx context.Context, ffmpegPath string, cmd []string, durMicros int64, onProgress func(delta int64)) (int64, error) {
	name := ffmpegPath
	if name == "" {
		name = "ffmpeg"
	}
	c := exec.Command(name, cmd...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // 后台静默运行，不弹命令行窗口
	stdout, err := c.StdoutPipe()
	if err != nil {
		return 0, err
	}
	stderrBuf := &LimitedBuffer{Max: 4096}
	c.Stderr = stderrBuf

	if err := c.Start(); err != nil {
		return 0, err
	}
	done := make(chan struct{})
	defer close(done)
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				if c.Process != nil {
					_ = c.Process.Kill()
				}
			case <-done:
			}
		}()
	}

	var outBytes int64
	lastMicro := int64(0)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "out_time_us="):
			if v, e := strconv.ParseInt(strings.TrimPrefix(line, "out_time_us="), 10, 64); e == nil && v > 0 {
				if v > lastMicro {
					lastMicro = v
					onProgress(v) // 累计已编码时长（微秒），由调用方换算百分比
				}
			}
		case strings.HasPrefix(line, "total_size="):
			if v, e := strconv.ParseInt(strings.TrimPrefix(line, "total_size="), 10, 64); e == nil {
				outBytes = v
			}
		}
	}
	scannerErr := scanner.Err()
	waitErr := c.Wait()
	if waitErr != nil {
		return outBytes, fmt.Errorf("%v\n%s", waitErr, strings.TrimSpace(stderrBuf.String()))
	}
	if scannerErr != nil {
		return outBytes, fmt.Errorf("读取进度失败: %v\n%s", scannerErr, strings.TrimSpace(stderrBuf.String()))
	}
	return outBytes, nil
}

// ProbeDuration 用 ffprobe 读取视频时长（微秒），失败返回 0
func ProbeDuration(ffprobePath, p string) int64 {
	name := ffprobePath
	if name == "" {
		name = "ffprobe"
	}
	c := exec.Command(name, "-v", "error", "-show_entries", "format=duration", "-of", "default=nw=1:nk=1", p)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := c.Output()
	if err != nil {
		return 0
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || f <= 0 {
		return 0
	}
	return int64(f * 1e6)
}

// ProbeInfo 返回视频的基础信息（时长秒、宽、高、fps）
func ProbeInfo(ffprobePath, p string) (durSec float64, width, height int, fps string) {
	name := ffprobePath
	if name == "" {
		name = "ffprobe"
	}
	c := exec.Command(name, "-v", "error",
		"-print_format", "json",
		"-show_entries", "format=duration:stream=width,height,avg_frame_rate,codec_type",
		p)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := c.Output()
	if err != nil {
		return 0, 0, 0, ""
	}
	// 简单解析 json 字段（不引入额外依赖）
	s := string(out)
	durSec = parseJSONFloat(s, "duration")
	width = parseJSONInt(s, "width")
	height = parseJSONInt(s, "height")
	fps = parseJSONString(s, "avg_frame_rate")
	return durSec, width, height, fps
}

// LimitedBuffer 限制收集 stderr 的长度，避免海量日志占内存
type LimitedBuffer struct {
	Buf []byte
	Max int
}

func (b *LimitedBuffer) Write(p []byte) (int, error) {
	if len(b.Buf) < b.Max {
		n := b.Max - len(b.Buf)
		if len(p) < n {
			n = len(p)
		}
		b.Buf = append(b.Buf, p[:n]...)
	}
	return len(p), nil
}

func (b *LimitedBuffer) String() string { return string(b.Buf) }

func extOf(p string) string {
	i := strings.LastIndexByte(p, '.')
	if i < 0 {
		return ""
	}
	return p[i:]
}

func parseJSONFloat(s, key string) float64 {
	v := parseJSONString(s, key)
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseJSONInt(s, key string) int {
	v := parseJSONString(s, key)
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

func parseJSONString(s, key string) string {
	idx := strings.Index(s, `"`+key+`"`)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(key)+2:]
	rest = strings.TrimLeft(rest, " \t\r\n:")
	if len(rest) == 0 {
		return ""
	}
	if rest[0] == '"' {
		end := strings.Index(rest[1:], `"`)
		if end < 0 {
			return ""
		}
		return rest[1 : 1+end]
	}
	end := strings.IndexAny(rest, ",}\n\r")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

// IsVideoFile 判断文件是否为支持的视频文件（排除 TypeScript 的 .d.ts 误判）
func IsVideoFile(p string) bool {
	lower := strings.ToLower(p)
	if strings.HasSuffix(lower, ".d.ts") {
		return false
	}
	return extSupported(filepath.Ext(lower))
}

func extSupported(ext string) bool {
	for _, e := range DefaultExts {
		if strings.EqualFold(ext, e) {
			return true
		}
	}
	return false
}

// SortedKeys 返回 map 的排序 key 列表
func SortedKeys(m map[string]bool) []string {
	r := make([]string, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	sort.Strings(r)
	return r
}
