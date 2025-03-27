package main

import (
    "context"
    "fmt"
    "log"
    "regexp"
    "strings"
    "testing"
    "time"
    "os"
    "github.com/chromedp/cdproto/dom"
    "github.com/chromedp/chromedp"
)

// 直接提取抖音无水印视频链接 - 简化版本
func extractDouyinNoWatermarkLinks(html string) []string {
    var links []string
    
    // 正则表达式直接匹配完整链接
    re := regexp.MustCompile(`(https?:)?//www\.douyin\.com/aweme/v1/play/\?[^"'\s]+`)
    matches := re.FindAllString(html, -1)
    
    for _, link := range matches {
        // 修复协议前缀
        if strings.HasPrefix(link, "//") {
            link = "https:" + link
        }
        
        links = append(links, link)
        fmt.Printf("找到抖音无水印链接: %s\n", link)
    }
    
    return links
}
func TestDLPandaWithChrome(t *testing.T) {
    // 创建一个带超时的上下文
    ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
    defer cancel()

    // 创建 Chrome 实例选项
    opts := append(chromedp.DefaultExecAllocatorOptions[:],
        chromedp.Flag("headless", true),
        chromedp.Flag("disable-gpu", true),
        chromedp.Flag("no-sandbox", true),
        chromedp.Flag("disable-dev-shm-usage", true),
        chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
    )

    // 创建一个新的浏览器实例
    allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
    defer cancel()

    // 创建一个新的浏览器上下文
    taskCtx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
    defer cancel()

    // 设置抖音视频URL
    douyinURL := "https://v.douyin.com/uSR4GjyWJUg/"
    // 创建目标URL (带查询参数)
    targetURL := fmt.Sprintf("https://www.dlpanda.com/?url=%s&token=%s", douyinURL, "G7eRpMaa")
    
    fmt.Printf("正在访问: %s\n", targetURL)

    // 等待浏览器完成初始化
    if err := chromedp.Run(taskCtx, chromedp.Navigate(targetURL)); err != nil {
        t.Fatalf("无法导航到页面: %v", err)
    }

    // 等待页面加载完成 (等待一个常见元素出现)
    if err := chromedp.Run(taskCtx, chromedp.WaitVisible(`body`, chromedp.ByQuery)); err != nil {
        t.Fatalf("等待页面加载失败: %v", err)
    }

    fmt.Println("页面已加载，等待解析完成...")

    // 等待一段时间，确保JavaScript执行完成
    time.Sleep(10 * time.Second)

    // 获取整个HTML内容
    var htmlContent string
    err := chromedp.Run(taskCtx, chromedp.ActionFunc(func(ctx context.Context) error {
        node, err := dom.GetDocument().Do(ctx)
        if err != nil {
            return err
        }
        htmlContent, err = dom.GetOuterHTML().WithNodeID(node.NodeID).Do(ctx)
        return err
    }))
    
    if err != nil {
        t.Fatalf("获取HTML内容失败: %v", err)
    }

    fmt.Printf("成功获取HTML内容，长度: %d 字节\n", len(htmlContent))
    
    // 保存HTML内容到文件（可选，用于调试）
    if err := os.WriteFile("panda_page.html", []byte(htmlContent), 0644); err != nil {
        fmt.Printf("保存HTML内容失败: %v\n", err)
    } else {
        fmt.Println("HTML内容已保存到 panda_page.html")
    }
    
    // 使用正则表达式提取抖音无水印链接
    noWatermarkLinks := extractDouyinNoWatermarkLinks(htmlContent)
    
    if len(noWatermarkLinks) > 0 {
        fmt.Printf("\n成功提取 %d 个无水印链接:\n", len(noWatermarkLinks))
        for i, link := range noWatermarkLinks {
            fmt.Printf("%d: %s\n", i+1, link)
        }
    } else {
        fmt.Println("\n未找到无水印链接，尝试使用备用方法...")
        
        // 添加备用提取方法
        backupPatterns := []string{
            // 匹配不同格式的视频链接
            `(https?:)?//[^"'\s]*douyin\.com/aweme/v1/play/[^"'\s]*`,
            `(https?:)?//[^"'\s]*amemv\.com/aweme/v1/play/[^"'\s]*`,
            `(https?:)?//[^"'\s]*\.mp4[^"'\s]*`,
            `data-src="([^"]+\.mp4[^"]*)"`,
            `href="([^"]+download[^"]*)"`,
        }
        
        fmt.Println("使用备用正则表达式模式:")
        
        var backupLinks []string
        for i, pattern := range backupPatterns {
            fmt.Printf("尝试模式 %d: %s\n", i+1, pattern)
            re := regexp.MustCompile(pattern)
            matches := re.FindAllStringSubmatch(htmlContent, -1)
            
            for _, match := range matches {
                link := match[0]
                // 如果正则表达式包含捕获组，使用第一个捕获组
                if len(match) > 1 && match[1] != "" {
                    link = match[1]
                }
                
                if strings.HasPrefix(link, "//") {
                    link = "https:" + link
                }
                
                backupLinks = append(backupLinks, link)
                fmt.Printf("找到潜在链接: %s\n", link)
            }
        }
        
        if len(backupLinks) > 0 {
            fmt.Printf("\n使用备用模式找到 %d 个潜在链接:\n", len(backupLinks))
            for i, link := range backupLinks {
                fmt.Printf("%d: %s\n", i+1, link)
            }
        } else {
            fmt.Println("使用备用模式仍未找到任何链接")
        }
    }
    
    // 截图以方便调试
    var buf []byte
    if err := chromedp.Run(taskCtx, chromedp.CaptureScreenshot(&buf)); err != nil {
        t.Fatalf("截图失败: %v", err)
    }
    
    if err := os.WriteFile("panda_screenshot.png", buf, 0644); err != nil {
        t.Fatalf("保存截图失败: %v", err)
    }
    fmt.Println("已保存页面截图到 panda_screenshot.png")
    // 强制报错
    t.Error("测试失败，强制报错")
}