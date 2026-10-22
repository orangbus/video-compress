// 命令行版：批量压缩文件夹内视频为 H.265 / CRF18，保留原分辨率与原帧率，带实时进度条。
// 用法示例:
//   go run ./cmd/cli -dir "D:\videos"                          # 默认 libx265 + fast 预设
//   go run ./cmd/cli -dir "." -encoder nvenc                   # 用 GPU (GTX1660+) 硬编提速
//   go run ./cmd/cli -dir "." -preset fast -concurrency 4      # 更快的软件编码
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"yasuo/internal/video"
)

type job struct {
	absPath   string
	outPath   string
	size      int64
	durMicros int64
}

// progressEvent 进度条事件
type progressEvent struct {
	delta int64
	file  string
	done  bool
	fail  bool
	inB   int64
	outB  int64
	err   string
}

func main() {
	dir := flag.String("dir", ".", "要压缩的视频所在目录(会递归查找)")
	out := flag.String("out", "", "输出目录(默认 <dir>/compressed)")
	crf := flag.Int("crf", 18, "H.265 画质参数，越小越清晰越大")
	preset := flag.String("preset", "fast", "libx265 预设: ultrafast|fast|medium|slow|veryslow(默认fast)")
	encoder := flag.String("encoder", "libx265", "编码器: libx265(CPU/CRF) 或 nvenc(GPU硬编, 需显卡支持HEVC)")
	extsStr := flag.String("ext", strings.Join(video.DefaultExts, ","), "要处理的扩展名，逗号分隔")
	concurrency := flag.Int("concurrency", 2, "并行压缩任务数")
	dryRun := flag.Bool("dry-run", false, "只打印将要执行的命令，不真正压缩")
	flag.Parse()

	exts := map[string]bool{}
	for _, e := range strings.Split(*extsStr, ",") {
		e = strings.TrimSpace(strings.ToLower(e))
		if e != "" {
			if !strings.HasPrefix(e, ".") {
				e = "." + e
			}
			exts[e] = true
		}
	}

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Println("无法解析目录:", err)
		os.Exit(1)
	}
	if st, err := os.Stat(absDir); err != nil || !st.IsDir() {
		fmt.Println("输入目录不存在或不是目录:", absDir)
		os.Exit(1)
	}

	outDir := *out
	if outDir == "" {
		outDir = filepath.Join(absDir, "compressed")
	}
	outAbs, _ := filepath.Abs(outDir)
	if *dryRun == false {
		if err := os.MkdirAll(outAbs, 0o755); err != nil {
			fmt.Println("无法创建输出目录:", err)
			os.Exit(1)
		}
	}

	// 1. 递归收集视频文件
	var jobs []job
	err = filepath.WalkDir(absDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.EqualFold(filepath.Clean(p), filepath.Clean(outAbs)) { // 跳过输出目录
				return filepath.SkipDir
			}
			return nil
		}
		lower := strings.ToLower(p)
		if strings.HasSuffix(lower, ".d.ts") { // 排除 TypeScript 声明文件误判
			return nil
		}
		if !exts[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		rel, _ := filepath.Rel(absDir, p)
		outPath := filepath.Join(outAbs, rel)
		if *dryRun == false {
			_ = os.MkdirAll(filepath.Dir(outPath), 0o755)
		}
		info, _ := d.Info()
		jobs = append(jobs, job{absPath: p, outPath: outPath, size: info.Size()})
		return nil
	})
	if err != nil {
		fmt.Println("遍历目录失败:", err)
		os.Exit(1)
	}

	if len(jobs) == 0 {
		fmt.Println("未找到任何视频文件。可扩展名:", video.SortedKeys(exts))
		os.Exit(0)
	}

	// 按大小排序，大的先处理
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].size > jobs[j].size })

	// 2. 探测每个视频的时长(用于进度条百分比)
	totalDur := int64(0)
	for i := range jobs {
		jobs[i].durMicros = video.ProbeDuration("", jobs[i].absPath)
		totalDur += jobs[i].durMicros
	}

	encName := *encoder
	if encName == "nvenc" {
		encName = "hevc_nvenc(GPU)"
	}
	fmt.Printf("输入目录 : %s\n输出目录 : %s\n找到视频 : %d 个\n编码器   : %s\nCRF      : %d\nPreset   : %s\n并行数   : %d\n\n",
		absDir, outAbs, len(jobs), encName, *crf, *preset, *concurrency)

	if *dryRun {
		for _, j := range jobs {
			fmt.Println("ffmpeg", strings.Join(video.BuildCmd(video.Job{AbsPath: j.absPath, OutPath: j.outPath}, *crf, *preset, *encoder), " "))
		}
		fmt.Println("(dry-run 结束，未执行压缩)")
		return
	}

	// 3. 进度条打印
	evtCh := make(chan progressEvent, 64)
	donePrint := make(chan struct{})
	isTTY := stdoutIsTTY()
	go printProgress(evtCh, donePrint, len(jobs), totalDur, isTTY)

	// 4. 并发压缩
	start := time.Now()
	var wg sync.WaitGroup
	sem := make(chan struct{}, *concurrency)
	ctx := context.Background()

	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			cmd := video.BuildCmd(video.Job{AbsPath: j.absPath, OutPath: j.outPath}, *crf, *preset, *encoder)
			outBytes, err := video.RunFFmpegProgress(ctx, "", cmd, j.durMicros, func(delta int64) {
				evtCh <- progressEvent{delta: delta, file: filepath.Base(j.absPath)}
			})
			if err != nil {
				evtCh <- progressEvent{done: true, fail: true, file: filepath.Base(j.absPath), err: err.Error()}
				return
			}
			evtCh <- progressEvent{done: true, file: filepath.Base(j.absPath), inB: j.size, outB: outBytes}
		}(j)
	}
	wg.Wait()
	close(evtCh)
	<-donePrint

	fmt.Printf("\n===== 汇总 =====\n")
	fmt.Printf("总耗时 %s\n", time.Since(start).Round(time.Second))
	fmt.Printf("输出目录: %s\n", outAbs)
}

// printProgress 消费事件并打印单行进度条（仅真实控制台刷新单行）
func printProgress(evtCh <-chan progressEvent, done chan<- struct{}, totalFiles int, totalDur int64, isTTY bool) {
	const barWidth = 30

	var doneMicro int64
	var doneCount, failCount int
	var totalIn, totalOut int64
	var current string
	lastShown := ""
	prevWidth := 0

	render := func() string {
		pct := 0.0
		if totalDur > 0 {
			pct = float64(doneMicro) / float64(totalDur) * 100
			if pct > 100 {
				pct = 100
			}
		}
		filled := int(pct / 100 * barWidth)
		if totalDur <= 0 {
			filled = int(float64(doneCount) / float64(totalFiles) * barWidth)
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		return fmt.Sprintf("[%s] %5.1f%% │ 完成 %d/%d 失败%d │ %s │ 输出 %.1f MB",
			bar, pct, doneCount, totalFiles, failCount, current, float64(totalOut)/1048576)
	}

	writeLine := func(line string, newline bool) {
		if line == lastShown {
			if newline {
				fmt.Println("\r" + line + strings.Repeat(" ", prevWidth-len(line)))
			}
			return
		}
		out := "\r" + line
		if pad := prevWidth - len(line); pad > 0 {
			out += strings.Repeat(" ", pad)
		}
		prevWidth = len(line)
		lastShown = line
		if newline {
			out += "\n"
		}
		fmt.Print(out)
	}

	for ev := range evtCh {
		switch {
		case !ev.done:
			doneMicro += ev.delta
			current = ev.file
			if isTTY {
				writeLine(render(), false)
			}
		case ev.fail:
			failCount++
			writeLine(render(), true)
			fmt.Printf("[失败] %s: %s\n", ev.file, ev.err)
		default:
			doneCount++
			totalIn += ev.inB
			totalOut += ev.outB
			ratio := 100.0
			if ev.inB > 0 {
				ratio = float64(ev.outB) / float64(ev.inB) * 100
			}
			writeLine(render(), true)
			fmt.Printf("  完成 %s: %.2f MB -> %.2f MB (%.1f%%)\n",
				ev.file, float64(ev.inB)/1048576, float64(ev.outB)/1048576, ratio)
		}
	}
	bar := strings.Repeat("█", barWidth)
	final := fmt.Sprintf("[%s] 100.0%% │ 完成 %d/%d 失败%d",
		bar, doneCount, totalFiles, failCount)
	if final != lastShown {
		fmt.Print("\r" + final)
	}
	fmt.Println()
	close(done)
}

func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
