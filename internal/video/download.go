package video

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FFmpegDownloadURLs 自动下载候选源（按顺序尝试）
var FFmpegDownloadURLs = []string{
	// gyan.dev 官方 essentials 版（自带 libx265 / nvenc）
	"https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip",
	// BtbN 每日构建 GPL 版（自带 libx265 / nvenc）
	"https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip",
}

// DownloadFFmpeg 下载并解压 ffmpeg.exe / ffprobe.exe 到 <destDir>/ffmpeg/bin/。
// onProgress 回调 (已下载字节, 总字节, 百分比)。
func DownloadFFmpeg(destDir string, onProgress func(downloaded, total int64, percent int)) (string, error) {
	var lastErr error
	for _, url := range FFmpegDownloadURLs {
		binDir, err := downloadAndExtract(url, destDir, onProgress)
		if err == nil {
			return binDir, nil
		}
		lastErr = err
	}
	return "", fmt.Errorf("所有下载源均失败，最后错误：%v", lastErr)
}

func downloadAndExtract(url, destDir string, onProgress func(downloaded, total int64, percent int)) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	tmp, err := os.CreateTemp("", "ffmpeg-dl-*.zip")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpPath)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) yasuo-video-compress")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败 HTTP %d (%s)", resp.StatusCode, url)
	}

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := tmp.Write(buf[:n]); werr != nil {
				return "", werr
			}
			downloaded += int64(n)
			pct := 0
			if total > 0 {
				pct = int(downloaded * 100 / total)
				if pct > 100 {
					pct = 100
				}
			}
			if onProgress != nil {
				onProgress(downloaded, total, pct)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	// 解压 ffmpeg.exe / ffprobe.exe（跳过 ffplay，节省空间）
	binDir := filepath.Join(destDir, "ffmpeg", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	zr, err := zip.OpenReader(tmpPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	foundFfmpeg, foundFfprobe := false, false
	for _, f := range zr.File {
		lower := strings.ToLower(f.Name)
		var target string
		switch {
		case strings.HasSuffix(lower, "/bin/ffmpeg.exe"):
			target = filepath.Join(binDir, "ffmpeg.exe")
			foundFfmpeg = true
		case strings.HasSuffix(lower, "/bin/ffprobe.exe"):
			target = filepath.Join(binDir, "ffprobe.exe")
			foundFfprobe = true
		default:
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, werr := io.Copy(out, rc)
		out.Close()
		rc.Close()
		if werr != nil {
			return "", werr
		}
	}
	if !foundFfmpeg || !foundFfprobe {
		return "", fmt.Errorf("压缩包内未找到 ffmpeg/ffprobe (%s)", url)
	}
	return binDir, nil
}
