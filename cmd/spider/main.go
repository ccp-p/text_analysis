package main

import (
    "flag"
    "fmt"
    "net/url"
    "os"
    "strings"
    "sync"
    "time"

    "golang.org/x/net/html"
    "net/http"
)

// 爬虫配置
type CrawlerConfig struct {
    StartURL   string
    MaxDepth   int
    MaxURLs    int
    SameHost   bool
    Timeout    time.Duration
    Concurrent int
}

// 页面数据
type PageData struct {
    URL      string
    Title    string
    Links    []string
    Depth    int
    Error    error
}

func main() {
    // 解析命令行参数
    startURL := flag.String("url", "https://go.dev/", "起始 URL")
    maxDepth := flag.Int("depth", 2, "最大爬取深度")
    maxURLs := flag.Int("max", 5, "最大爬取 URL 数量")
    sameHost := flag.Bool("same-host", true, "仅爬取相同主机的 URL")
    timeout := flag.Duration("timeout", 10*time.Second, "HTTP 请求超时")
    concurrent := flag.Int("concurrent", 5, "并发爬取数量")
    outputFile := flag.String("output", "", "输出结果到文件")
    flag.Parse()

    // 验证起始 URL
    if _, err := url.Parse(*startURL); err != nil {
        fmt.Printf("无效的 URL: %v\n", err)
        os.Exit(1)
    }

    // 创建爬虫配置
    config := CrawlerConfig{
        StartURL:   *startURL,
        MaxDepth:   *maxDepth,
        MaxURLs:    *maxURLs,
        SameHost:   *sameHost,
        Timeout:    *timeout,
        Concurrent: *concurrent,
    }

    // 开始爬取
    fmt.Printf("开始从 %s 爬取网页 (最大深度: %d, 最大 URL 数: %d)\n",
        config.StartURL, config.MaxDepth, config.MaxURLs)

    startTime := time.Now()
    results := crawl(config)
    elapsed := time.Since(startTime)

    // 显示结果
    fmt.Printf("\n爬取完成! 共爬取 %d 个页面, 耗时: %v\n", len(results), elapsed)

    // 如果指定了输出文件，将结果写入文件
    if *outputFile != "" {
        if err := writeResults(*outputFile, results); err != nil {
            fmt.Printf("写入结果失败: %v\n", err)
        } else {
            fmt.Printf("结果已保存到: %s\n", *outputFile)
        }
    } else {
        // 在终端显示结果
        displayResults(results)
    }
}

// 爬取网页
func crawl(config CrawlerConfig) []PageData {
    startURL, _ := url.Parse(config.StartURL)
    baseHost := startURL.Host

    // 存储已访问的 URL
    visited := make(map[string]bool)
    visitedMutex := sync.Mutex{}

    // 存储结果
    var results []PageData
    resultsMutex := sync.Mutex{}

    // 创建爬取队列和等待组
    queue := make(chan PageData, config.MaxURLs)
    var wg sync.WaitGroup

    // 添加起始 URL
    queue <- PageData{URL: config.StartURL, Depth: 0}
    visited[config.StartURL] = true

    // 启动工作协程
    for i := 0; i < config.Concurrent; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()

            for {
                // 检查队列
                var page PageData
                var more bool

                select {
                case page, more = <-queue:
                    if !more {
                        return
                    }
                default:
                    // 队列为空，检查是否还有工作在进行
                    visitedMutex.Lock()
                    count := len(visited)
                    visitedMutex.Unlock()

                    resultsMutex.Lock()
                    resCount := len(results)
                    resultsMutex.Unlock()

                    if count >= config.MaxURLs || resCount >= config.MaxURLs {
                        return
                    }
                    
                    // 等待队列中的数据
                    page, more = <-queue
                    if !more {
                        return
                    }
                }

                // 检查深度限制
                if page.Depth > config.MaxDepth {
                    continue
                }

                // 爬取页面
                pageData := fetchPage(page.URL, config.Timeout)
                pageData.Depth = page.Depth

                // 保存结果
                resultsMutex.Lock()
                if len(results) < config.MaxURLs {
                    results = append(results, pageData)
                    fmt.Printf("\r已爬取 %d/%d 个页面", len(results), config.MaxURLs)
                }
                resultsMutex.Unlock()

                // 如果达到最大 URL 数，关闭队列
                resultsMutex.Lock()
                if len(results) >= config.MaxURLs {
                    resultsMutex.Unlock()
                    return
                }
                resultsMutex.Unlock()

                // 如果有错误，不继续处理链接
                if pageData.Error != nil {
                    continue
                }

                // 处理页面中的链接
                for _, link := range pageData.Links {
                    linkURL, err := url.Parse(link)
                    if err != nil {
                        continue
                    }

                    // 处理相对 URL
                    if !linkURL.IsAbs() {
                        baseURL, _ := url.Parse(page.URL)
                        linkURL = baseURL.ResolveReference(linkURL)
                    }

                    absLink := linkURL.String()

                    // 跳过非 HTTP/HTTPS 链接
                    if !strings.HasPrefix(absLink, "http") {
                        continue
                    }

                    // 检查是否应该仅爬取相同主机
                    if config.SameHost && linkURL.Host != baseHost {
                        continue
                    }

                    // 检查是否已访问
                    visitedMutex.Lock()
                    if !visited[absLink] && len(visited) < config.MaxURLs {
                        visited[absLink] = true
                        queue <- PageData{URL: absLink, Depth: page.Depth + 1}
                    }
                    visitedMutex.Unlock()
                }
            }
        }()
    }

    // 等待队列变空
    go func() {
        wg.Wait()
        close(queue)
    }()

    // 等待所有工作完成
    wg.Wait()

    return results
}

// 获取页面数据
func fetchPage(url string, timeout time.Duration) PageData {
    client := &http.Client{
        Timeout: timeout,
    }

    resp, err := client.Get(url)
    if err != nil {
        return PageData{URL: url, Error: err}
    }
    defer resp.Body.Close()

    // 解析 HTML
    doc, err := html.Parse(resp.Body)
    if err != nil {
        return PageData{URL: url, Error: err}
    }

    // 提取标题和链接
    pageData := PageData{URL: url}
    pageData.Title = extractTitle(doc)
    pageData.Links = extractLinks(doc)

    return pageData
}

// 提取页面标题
func extractTitle(n *html.Node) string {
    if n.Type == html.ElementNode && n.Data == "title" {
        if n.FirstChild != nil {
            return n.FirstChild.Data
        }
        return ""
    }

    for c := n.FirstChild; c != nil; c = c.NextSibling {
        if title := extractTitle(c); title != "" {
            return title
        }
    }

    return ""
}

// 提取页面链接
func extractLinks(n *html.Node) []string {
    var links []string

    var extractFunc func(*html.Node)
    extractFunc = func(n *html.Node) {
        if n.Type == html.ElementNode && n.Data == "a" {
            for _, a := range n.Attr {
                if a.Key == "href" {
                    links = append(links, a.Val)
                    break
                }
            }
        }

        for c := n.FirstChild; c != nil; c = c.NextSibling {
            extractFunc(c)
        }
    }

    extractFunc(n)
    return links
}

// 显示爬取结果
func displayResults(results []PageData) {
    fmt.Println("\n爬取结果:")
    for i, page := range results {
        fmt.Printf("%d. %s\n", i+1, page.URL)
        fmt.Printf("   标题: %s\n", page.Title)
        fmt.Printf("   深度: %d\n", page.Depth)
        if page.Error != nil {
            fmt.Printf("   错误: %v\n", page.Error)
        } else {
            fmt.Printf("   链接数: %d\n", len(page.Links))
        }
        fmt.Println()
    }
}

// 将结果写入文件
func writeResults(filename string, results []PageData) error {
    file, err := os.Create(filename)
    if err != nil {
        return err
    }
    defer file.Close()

    // 写入CSV格式的标题
    _, err = fmt.Fprintln(file, "URL,标题,深度,链接数,错误")
    if err != nil {
        return err
    }

    // 写入数据
    for _, page := range results {
        // 处理CSV中的特殊字符
        title := strings.ReplaceAll(page.Title, "\"", "\"\"")
        var errorStr string
        if page.Error != nil {
            errorStr = strings.ReplaceAll(page.Error.Error(), "\"", "\"\"")
        }

        _, err := fmt.Fprintf(file, "\"%s\",\"%s\",%d,%d,\"%s\"\n",
            page.URL, title, page.Depth, len(page.Links), errorStr)
        if err != nil {
            return err
        }
    }

    return nil
}