package pkg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

var ffmpegBinaryPath string

// CheckFFmpegInstalled 检查 FFmpeg 是否已安装
func CheckFFmpegInstalled() bool {
	if ffmpegBinaryPath != "" {
		return true
	}

	// 首先尝试在 PATH 中查找
	path, err := exec.LookPath("ffmpeg")
	if err == nil {
		fmt.Printf("CheckFFmpegInstalled: ffmpeg found at %s\n", path)
		ffmpegBinaryPath = path
		return true
	}

	// 尝试在当前可执行文件所在目录查找（适用于桌面端/打包场景）
	if exe, exeErr := os.Executable(); exeErr == nil {
		exeDir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(exeDir, "ffmpeg"),
			filepath.Join(exeDir, "ffmpeg.exe"),
			filepath.Join(exeDir, "bin", "ffmpeg"),
			filepath.Join(exeDir, "bin", "ffmpeg.exe"),
		}
		for _, p := range candidates {
			if _, statErr := os.Stat(p); statErr == nil {
				fmt.Printf("CheckFFmpegInstalled: ffmpeg found at %s\n", p)
				ffmpegBinaryPath = p
				return true
			}
		}
	}

	// 如果 PATH 中没找到，尝试常见路径
	commonPaths := []string{
		"/usr/bin/ffmpeg",
		"/usr/local/bin/ffmpeg",
		"/opt/homebrew/bin/ffmpeg",
		"/snap/bin/ffmpeg",
	}

	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			fmt.Printf("CheckFFmpegInstalled: ffmpeg found at %s\n", p)
			ffmpegBinaryPath = p
			return true
		}
	}

	fmt.Printf("CheckFFmpegInstalled: ffmpeg not found: %v\n", err)
	return false
}

// CompressImageWithFFmpeg 使用 FFmpeg 压缩图片
func CompressImageWithFFmpeg(inputData []byte, ext string) ([]byte, error) {
	if !CheckFFmpegInstalled() {
		return inputData, nil // FFmpeg 未安装，跳过压缩
	}

	// 创建临时文件用于输入
	tmpInput, err := os.CreateTemp("", "upload-img-*"+ext)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpInput.Name())
	defer tmpInput.Close()

	if _, err := tmpInput.Write(inputData); err != nil {
		tmpInput.Close()
		return nil, err
	}
	tmpInput.Close()

	// 创建临时文件用于输出
	tmpOutput := tmpInput.Name() + "_compressed" + ext
	defer os.Remove(tmpOutput)

	// 构建 FFmpeg 命令
	// -y: 覆盖输出文件
	// -q:v 20: 质量控制 (2-31, 31最差)
	args := []string{"-y", "-i", tmpInput.Name()}

	// 根据格式调整参数
	ext = strings.ToLower(ext)
	if ext == ".jpg" || ext == ".jpeg" {
		args = append(args, "-q:v", "15") // JPEG 质量
	} else if ext == ".png" {
		// PNG 压缩通常使用 -compression_level 或调色板，这里简单处理
		// ffmpeg 处理 PNG 压缩效果可能不如 pngquant，但这里只用 ffmpeg
		args = append(args, "-q:v", "15")
	} else if ext == ".webp" {
		args = append(args, "-q:v", "50") // WebP 质量 (0-100)
	}

	args = append(args, tmpOutput)

	cmd := exec.Command(ffmpegBinaryPath, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg error: %v, output: %s", err, string(output))
	}

	// 读取压缩后的数据
	compressedData, err := os.ReadFile(tmpOutput)
	if err != nil {
		return nil, err
	}

	// 如果压缩后体积没有变小（甚至更大），直接返回原始数据，避免“压缩几乎无效果”的体验
	if len(compressedData) >= len(inputData) {
		return inputData, nil
	}

	return compressedData, nil
}

func CompressVideoWithFFmpeg(inputPath string) (string, error) {
	if !CheckFFmpegInstalled() {
		return "", fmt.Errorf("ffmpeg not installed")
	}

	// 创建临时文件用于输出
	// 强制使用 .mp4 扩展名，因为我们使用 libx264 + aac 编码，这是最通用的 Web 格式
	outputExt := ".mp4"
	tmpOutput := filepath.Join(os.TempDir(), "compressed-"+uuid.New().String()+outputExt)

	// 构建 FFmpeg 命令
	// -vf scale: 对超大视频进行降分辨率以获得更明显的压缩收益
	// -crf 30: 更激进的压缩（文件更小）
	// -preset medium: 比 fast 更小，但 CPU 消耗更高
	// -b:a 96k: 音频更小
	// -pix_fmt yuv420p: 兼容性更好
	// -movflags +faststart: 优化 Web 播放（元数据移到文件头）
	cmd := exec.Command(ffmpegBinaryPath, "-y", "-i", inputPath,
		"-vf", "scale='min(1280,iw)':-2",
		"-vcodec", "libx264", "-crf", "30", "-preset", "medium",
		"-pix_fmt", "yuv420p",
		"-acodec", "aac", "-b:a", "96k",
		"-movflags", "+faststart",
		tmpOutput)

	if output, err := cmd.CombinedOutput(); err != nil {
		os.Remove(tmpOutput) // 清理失败的输出
		return "", fmt.Errorf("ffmpeg error: %v, output: %s", err, string(output))
	}

	// 如果压缩后体积没有变小（甚至更大），放弃压缩结果
	inStat, inErr := os.Stat(inputPath)
	outStat, outErr := os.Stat(tmpOutput)
	if inErr == nil && outErr == nil {
		if outStat.Size() >= inStat.Size() {
			os.Remove(tmpOutput)
			return "", nil
		}
	}

	return tmpOutput, nil
}
