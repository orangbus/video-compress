package video

import (
	"testing"
)

// TestDetectEnv 仅做冒烟验证：环境检测不崩溃且关键字段合理（不依赖具体硬件）
func TestDetectEnv(t *testing.T) {
	env := DetectEnv()
	t.Logf("CPU=%s cores=%d mem=%dGB GPU=%s nvenc=%v ffmpeg=%s ver=%s local=%v ready=%v",
		env.CPUModel, env.CPUCores, env.MemoryGB, env.GPUName, env.NVENCHEVC,
		env.FFmpegPath, env.FFmpegVer, env.FFmpegLocal, env.Ready)

	if env.CPUCores <= 0 {
		t.Error("CPU cores should be > 0")
	}
	if env.MemoryGB <= 0 {
		t.Error("memory should be > 0")
	}
}
