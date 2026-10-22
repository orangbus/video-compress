package video

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// EnvInfo 电脑配置与 ffmpeg 环境信息（用于开始页展示与模式推荐）
type EnvInfo struct {
	CPUModel    string `json:"cpuModel"`
	CPUCores    int    `json:"cpuCores"`  // 逻辑核数
	MemoryGB    int    `json:"memoryGB"`  // 总物理内存(GB)
	GPUName     string `json:"gpuName"`
	NVENCHEVC   bool   `json:"nvencHevc"` // 是否支持 HEVC 硬编
	FFmpegPath  string `json:"ffmpegPath"`
	FFprobePath string `json:"ffprobePath"`
	FFmpegVer   string `json:"ffmpegVer"`
	FFmpegLocal bool   `json:"ffmpegLocal"` // ffmpeg 是否位于软件目录（自带）
	ExeDir      string `json:"exeDir"`      // 软件当前目录
	Ready       bool   `json:"ready"`       // 是否具备开始压缩的条件
}

// DetectEnv 检测本机环境（每次调用重新检测，耗时约 1~2 秒）
func DetectEnv() EnvInfo {
	exeDir := ExeDir()
	info := EnvInfo{
		ExeDir:    exeDir,
		CPUCores:  runtime.NumCPU(),
		MemoryGB:  memoryGB(),
		CPUModel:  cpuModel(),
		GPUName:   gpuName(),
		FFmpegPath: findFFmpeg(exeDir),
	}

	if info.FFmpegPath != "" {
		info.FFmpegVer = ffmpegVersion(info.FFmpegPath)
		info.FFprobePath = ffprobeNear(info.FFmpegPath)
		info.FFmpegLocal = isLocalBundled(info.FFmpegPath, exeDir)
		info.NVENCHEVC = strings.Contains(strings.ToLower(info.GPUName), "nvidia") && hasEncoder(info.FFmpegPath, "hevc_nvenc")
	}
	info.Ready = info.FFmpegPath != "" && info.FFprobePath != ""
	return info
}

// ExeDir 返回可执行文件所在目录（自带 ffmpeg 也放这里）
func ExeDir() string {
	ex, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(ex)
}

// findFFmpeg 依次查找：软件目录自带(ffmpeg/bin) -> PATH
func findFFmpeg(exeDir string) string {
	// 1) 软件目录自带：<exeDir>/ffmpeg/bin/ffmpeg.exe
	local := filepath.Join(exeDir, "ffmpeg", "bin", "ffmpeg.exe")
	if st, err := os.Stat(local); err == nil && !st.IsDir() {
		return local
	}
	// 2) PATH
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		abs, _ := filepath.Abs(p)
		return abs
	}
	return ""
}

// ffprobeNear 返回与 ffmpeg 同目录的 ffprobe
func ffprobeNear(ffmpegPath string) string {
	dir := filepath.Dir(ffmpegPath)
	p := filepath.Join(dir, "ffprobe.exe")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	// 退而求其次：PATH 中的 ffprobe
	if p2, err := exec.LookPath("ffprobe"); err == nil {
		abs, _ := filepath.Abs(p2)
		return abs
	}
	return ""
}

func isLocalBundled(ffmpegPath, exeDir string) bool {
	local := filepath.Join(exeDir, "ffmpeg", "bin", "ffmpeg.exe")
	return strings.EqualFold(ffmpegPath, local)
}

func ffmpegVersion(ffmpegPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, ffmpegPath, "-version")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := c.Output()
	if err != nil {
		return ""
	}
	first := strings.SplitN(string(out), "\n", 2)[0]
	return strings.TrimSpace(first)
}

// hasEncoder 检查 ffmpeg 是否包含指定编码器
func hasEncoder(ffmpegPath, enc string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, ffmpegPath, "-hide_banner", "-encoders")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := c.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), enc)
}

// cpuModel 从注册表读取 CPU 型号
func cpuModel() string {
	k, err := openRegKey(`HARDWARE\DESCRIPTION\System\CentralProcessor\0`)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetStringValue("ProcessorNameString")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(v)
}

// memoryGB 读取总物理内存(GB)
func memoryGB() int {
	kernel32 := newLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GlobalMemoryStatusEx")
	type memStatusEx struct {
		Length               uint32
		MemoryLoad           uint32
		TotalPhys            uint64
		AvailPhys            uint64
		TotalPageFile        uint64
		AvailPageFile        uint64
		TotalVirtual         uint64
		AvailVirtual         uint64
		AvailExtendedVirtual uint64
	}
	var ms memStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	r, _, _ := proc.Call(uintptr(unsafe.Pointer(&ms)))
	if r == 0 {
		return 0
	}
	return int(ms.TotalPhys / (1024 * 1024 * 1024))
}

// gpuName 查询显卡名称（PowerShell CIM，耗时约 1s）
func gpuName() string {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	ps := findPowerShell()
	if ps == "" {
		return ""
	}
	c := exec.CommandContext(ctx, ps, "-NoProfile", "-NonInteractive",
		"-Command", "Get-CimInstance Win32_VideoController | Select-Object -ExpandProperty Name")
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := c.Output()
	if err != nil {
		return ""
	}
	names := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return strings.Join(names, " / ")
}

// findPowerShell 定位可用的 PowerShell 可执行文件
func findPowerShell() string {
	for _, name := range []string{"powershell", "pwsh"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	// 兜底：Windows PowerShell 5.1 固定路径
	fixed := `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`
	if _, err := os.Stat(fixed); err == nil {
		return fixed
	}
	return ""
}
