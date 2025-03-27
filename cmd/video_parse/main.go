package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// 视频信息结构体
type VideoInfo struct {
	Title    string `json:"title"`
	Cover    string `json:"cover"`
	VideoURL string `json:"video_url"`
	Author   string `json:"author"`
	Platform string `json:"platform"`
}

// 解析抖音短链接
func ParseDouyinShortURL(shortURL string) (*VideoInfo, error) {
	// 1. 处理短链接，确保格式正确
	shortURL = extractURL(shortURL)
	if shortURL == "" {
		return nil, fmt.Errorf("无法从文本中提取有效链接")
	}

	fmt.Printf("提取到的短链接: %s\n", shortURL)

	// 2. 设置HTTP客户端，跟随重定向获取真实链接
	client := &http.Client{
		Timeout: 30 * time.Second, // 增加超时时间
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("过多重定向")
			}
			// 复制所有头部到重定向请求
			for key, values := range via[0].Header {
				for _, value := range values {
					req.Header.Add(key, value)
				}
			}
			return nil
		},
	}

	// 4. 如果API方法失败，使用传统的重定向方法
	return tryRedirectMethod(shortURL, client)
}

// 从文本中提取URL
func extractURL(text string) string {
	re := regexp.MustCompile(`https?://[^\s]+`)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 0 {
		// 清理URL末尾可能的非URL字符
		url := matches[0]
		url = regexp.MustCompile(`[,.;\s]+$`).ReplaceAllString(url, "")
		return url
	}
	return ""
}

// 将带水印URL转换为无水印URL
func convertToNoWatermarkURL(watermarkedURL string) string {
	// 检查URL是否为空
	if watermarkedURL == "" {
		return ""
	}

	// 提取video_id参数
	videoIDRegex := regexp.MustCompile(`video_id=([^&]+)`)
	matches := videoIDRegex.FindStringSubmatch(watermarkedURL)

	if len(matches) < 2 {
		// 如果找不到video_id，尝试从路径中提取
		pathRegex := regexp.MustCompile(`/([^/]+)\.mp4`)
		matches = pathRegex.FindStringSubmatch(watermarkedURL)
		if len(matches) < 2 {
			// 如果仍然找不到，返回原始URL
			fmt.Println("无法从URL中提取视频ID，返回原始URL")
			return watermarkedURL
		}
	}

	videoID := matches[1]
	fmt.Printf("提取到video_id: %s\n", videoID)

	// 构建无水印URL
	noWatermarkURL := fmt.Sprintf("https://www.douyin.com/aweme/v1/play/?video_id=%s&ratio=720p&line=0", videoID)

	return noWatermarkURL
}

// 使用传统重定向方法
func tryRedirectMethod(shortURL string, client *http.Client) (*VideoInfo, error) {
	// 发送请求获取重定向后的真实URL
	req, err := http.NewRequest("GET", shortURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 15_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.0 Mobile/15E148 Safari/604.1")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 获取真实URL
	realURL := resp.Request.URL.String()
	fmt.Printf("重定向后的真实URL: %s\n", realURL)

	// 尝试从URL中提取视频ID
	var videoID string
	patterns := []string{
		`/video/(\d+)/?`,
		`/share/video/(\d+)/?`,
		`/share/slides/(\d+)/?`, // 新增：处理 /share/slides/ 格式
		`item_id=(\d+)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(realURL)
		if len(matches) > 1 {
			videoID = matches[1]
			break
		}
	}

	if videoID == "" {
		return nil, fmt.Errorf("无法从URL中提取视频ID")
	}

	fmt.Printf("提取的视频ID: %s\n", videoID)

	// 读取HTML内容用于备用解析
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	htmlContent := string(body)

	// 尝试从页面中找到隐藏的JSON数据
	var jsonData map[string]interface{}

	jsonPatterns := []string{
		`<script id="RENDER_DATA" type="application/json">([^<]+)</script>`,
		`<script [^>]*id="__NEXT_DATA__"[^>]*>([^<]+)</script>`,
		`<script [^>]*id="__MODERN_SERVER_DATA__"[^>]*>([^<]+)</script>`,
		`window\.__INIT_PROPS__\s*=\s*({[^<]+});?</script>`,
	}

	for _, pattern := range jsonPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(htmlContent)
		if len(matches) > 1 {
			jsonStr := matches[1]
			// 有些JSON数据可能是URL编码的
			jsonStr, _ = url.QueryUnescape(jsonStr)

			if err := json.Unmarshal([]byte(jsonStr), &jsonData); err == nil {
				fmt.Println("成功解析页面JSON数据")
				break
			}
		}
	}

	// 尝试从JSON数据中提取
	if jsonData != nil {
		videoInfo := extractFromJSON(jsonData)
		if videoInfo.VideoURL != "" {
			videoInfo.Platform = "douyin"
			return videoInfo, nil
		}
	}

	// 最后尝试从HTML中直接提取
	var title, author, cover, videoURL string
	regexPatterns := []struct {
		name    string
		pattern string
		field   *string
	}{
		{"视频URL", `"playAddr":\s*"([^"]+)"`, &videoURL},
		{"视频URL备选", `"play_addr":\s*\{[^}]*"url_list":\s*\["([^"]+)"`, &videoURL},
		{"标题", `"desc":\s*"([^"]+)"`, &title},
		{"作者", `"nickname":\s*"([^"]+)"`, &author},
		{"封面", `"cover":\s*"([^"]+)"`, &cover},
		{"封面备选", `"origin_cover":\s*\{[^}]*"url_list":\s*\["([^"]+)"`, &cover},
	}

	// 修改 tryRedirectMethod 函数中从HTML提取URL后的代码部分
	for _, p := range regexPatterns {
		re := regexp.MustCompile(p.pattern)
		matches := re.FindStringSubmatch(htmlContent)
		if len(matches) > 1 {
			*p.field = strings.ReplaceAll(matches[1], "\\u002F", "/")
			fmt.Printf("从HTML找到 %s: %s\n", p.name, *p.field)

			// 如果是视频URL，尝试转换为无水印URL
			if p.field == &videoURL {
				originalURL := *p.field
				noWatermarkURL := convertToNoWatermarkURL(originalURL)
				if noWatermarkURL != originalURL {
					*p.field = noWatermarkURL
					fmt.Printf("转换为无水印URL: %s\n", *p.field)
				}
			}
		}
	}

	if videoURL == "" {
		return nil, fmt.Errorf("通过所有方法均未能提取到视频URL")
	}

	return &VideoInfo{
		Title:    title,
		Cover:    cover,
		VideoURL: videoURL,
		Author:   author,
		Platform: "douyin",
	}, nil
}

func extractFromJSON(data map[string]interface{}) *VideoInfo {
	result := &VideoInfo{}

	// 查找视频URL (多种可能的键)
	urlKeys := []string{"playAddr", "play_addr", "url", "download_addr", "download_url"}
	for _, key := range urlKeys {
		findInJSON(data, key, func(val interface{}) {
			switch v := val.(type) {
			case string:
				if result.VideoURL == "" {
					result.VideoURL = strings.ReplaceAll(v, "\\u002F", "/")
				}
			case map[string]interface{}:
				if urlList, ok := v["url_list"].([]interface{}); ok && len(urlList) > 0 {
					if url, ok := urlList[0].(string); ok && result.VideoURL == "" {
						result.VideoURL = strings.ReplaceAll(url, "\\u002F", "/")
					}
				}
			}
		})
		if result.VideoURL != "" {
			break
		}
	}

	// 查找描述和标题 (多种可能的键)
	titleKeys := []string{"desc", "title", "content", "text"}
	for _, key := range titleKeys {
		findInJSON(data, key, func(val interface{}) {
			if title, ok := val.(string); ok && title != "" && result.Title == "" {
				result.Title = title
			}
		})
		if result.Title != "" {
			break
		}
	}

	// 查找作者
	findInJSON(data, "nickname", func(val interface{}) {
		if name, ok := val.(string); ok {
			result.Author = name
		}
	})

	// 查找封面图
	findInJSON(data, "cover", func(val interface{}) {
		if url, ok := val.(string); ok {
			result.Cover = strings.ReplaceAll(url, "\\u002F", "/")
		}
	})

	return result
}

// 递归查找JSON中的特定键
func findInJSON(data interface{}, key string, callback func(interface{})) {
	switch v := data.(type) {
	case map[string]interface{}:
		for k, val := range v {
			if k == key {
				callback(val)
			} else {
				findInJSON(val, key, callback)
			}
		}
	case []interface{}:
		for _, val := range v {
			findInJSON(val, key, callback)
		}
	}
}

// 下载视频文件
func downloadVideo(videoURL, outputPath string) error {
	fmt.Printf("开始下载视频: %s\n", videoURL)

	// 创建输出目录
	dir := filepath.Dir(outputPath)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建输出目录失败: %w", err)
		}
	}

	// 创建请求
	req, err := http.NewRequest("GET", videoURL, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败: %w", err)
	}

	// 设置用户代理
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 15_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/15.0 Mobile/15E148 Safari/604.1")
	req.Header.Set("Referer", "https://www.douyin.com/")

	// 创建HTTP客户端
	client := &http.Client{
		Timeout: 5 * time.Minute, // 下载可能需要更长时间
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回非成功状态码: %d", resp.StatusCode)
	}

	// 创建输出文件
	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建输出文件失败: %w", err)
	}
	defer out.Close()

	// 创建进度条
	total := resp.ContentLength
	count := int64(0)
	lastProgressTime := time.Now()

	// 创建缓冲区
	buf := make([]byte, 32*1024) // 32KB 缓冲区

	// 复制数据到文件
	for {
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			// 写入到文件
			nw, ew := out.Write(buf[0:nr])
			if nw > 0 {
				count += int64(nw)

				// 每秒更新一次进度
				if time.Since(lastProgressTime) > time.Second {
					if total > 0 {
						fmt.Printf("\r下载进度: %.2f%% (%d/%d 字节)", float64(count)/float64(total)*100, count, total)
					} else {
						fmt.Printf("\r下载进度: %d 字节", count)
					}
					lastProgressTime = time.Now()
				}
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			break
		}
	}

	fmt.Println() // 换行

	if err != nil {
		return fmt.Errorf("下载过程中出错: %w", err)
	}

	fmt.Printf("视频下载完成: %s\n", outputPath)
	return nil
}

// 打开视频文件
func openFile(path string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", path)
	case "darwin": // macOS
		cmd = exec.Command("open", path)
	default: // Linux and others
		cmd = exec.Command("xdg-open", path)
	}

	return cmd.Start()
}

// 清理文件名，移除不合法字符
func sanitizeFilename(filename string) string {
	// 替换不允许作为文件名的字符
	illegal := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := filename

	for _, char := range illegal {
		result = strings.ReplaceAll(result, char, "_")
	}

	// 限制长度
	if len(result) > 100 {
		result = result[:100]
	}

	return strings.TrimSpace(result)
}

func main() {
	var shortURL string

	if len(os.Args) > 1 {
		shortURL = os.Args[1]
	} else {
		// 如果没有命令行参数，询问输入
		fmt.Print("请粘贴抖音分享文本或链接: ")

		// shortURL = "https://v.douyin.com/uSR4GjyWJUg/"
		shortURL = "https://v.douyin.com/QE8OSEZQ7e4"
	}

	fmt.Println("正在解析抖音链接...")

	// 设置随机种子
	rand.Seed(time.Now().UnixNano())

	// 解析视频信息
	videoInfo, err := ParseDouyinShortURL(shortURL)
	if err != nil {
		fmt.Printf("解析失败: %v\n", err)
		// 等待用户按回车退出
		fmt.Println("按回车键退出...")
		fmt.Scanln()
		os.Exit(1)
	}

	fmt.Println("\n==== 解析结果 ====")
	fmt.Printf("标题: %s\n", videoInfo.Title)
	fmt.Printf("作者: %s\n", videoInfo.Author)
	fmt.Printf("封面: %s\n", videoInfo.Cover)
	fmt.Printf("视频URL: %s\n\n", videoInfo.VideoURL)

	// 生成输出文件名
	title := sanitizeFilename(videoInfo.Title)
	if title == "" {
		title = "抖音视频_" + time.Now().Format("20060102150405")
	}
	outputPath := title + ".mp4"

	// 询问用户是否下载
	fmt.Printf("是否下载此视频? (y/n): ")
	var choice string
	fmt.Scanln(&choice)

	if strings.ToLower(choice) == "y" || strings.ToLower(choice) == "yes" {
		// 下载视频
		if err := downloadVideo(videoInfo.VideoURL, outputPath); err != nil {
			fmt.Printf("下载失败: %v\n", err)
		} else {
			// 询问是否打开视频
			fmt.Printf("是否立即播放视频? (y/n): ")
			fmt.Scanln(&choice)

			if strings.ToLower(choice) == "y" || strings.ToLower(choice) == "yes" {
				if err := openFile(outputPath); err != nil {
					fmt.Printf("无法打开视频: %v\n", err)
				}
			}
		}
	}

	fmt.Println("操作完成，按回车键退出...")
	fmt.Scanln()
}
