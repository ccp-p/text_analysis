package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// 搜索结果
type Result struct {
    File    string
    Line    int
    Content string
}

// 过滤器配置
type FilterConfig struct {
    Pattern     string // 正则表达式模式
    Extensions  []string // 文件扩展名
    IgnoreDirs  []string // 忽略的目录
    MaxFileSize int64 // 最大文件大小(字节)
}

func main() {
    // 命令行参数
    rootDir := flag.String("dir", "..//..//..", "要搜索的根目录")
    pattern := flag.String("pattern", "Test", "要搜索的正则表达式模式")
    extensions := flag.String("ext", ".go,.js,.py,.html,.css", "要搜索的文件扩展名(逗号分隔)")
    ignoreDirs := flag.String("ignore", "node_modules,vendor,.git", "要忽略的目录(逗号分隔)")
    concurrency := flag.Int("concurrency", runtime.NumCPU(), "并发处理的文件数")
    maxSize := flag.Int64("maxsize", 10*1024*1024, "最大文件大小(字节)")
    flag.Parse()

    if *pattern == "" {
        fmt.Println("请指定搜索模式，使用 -pattern 参数")
        flag.Usage()
        return
    }

    // 创建过滤器配置
    config := FilterConfig{
        Pattern:     *pattern,
        Extensions:  strings.Split(*extensions, ","),
        IgnoreDirs:  strings.Split(*ignoreDirs, ","),
        MaxFileSize: *maxSize,
    }

    // 编译正则表达式
    regex, err := regexp.Compile(*pattern)
    if err != nil {
        fmt.Printf("正则表达式编译错误: %v\n", err)
        return
    }

    // 开始计时
    startTime := time.Now()

    // 收集要处理的文件
    fmt.Println("正在收集文件...")
    files := collectFiles(*rootDir, config)
    fmt.Printf("找到 %d 个符合条件的文件\n", len(files))

    // 并行处理文件
    fmt.Printf("使用 %d 个并发工作器开始搜索...\n", *concurrency)
    results := searchFilesParallel(files, regex, *concurrency)

    // 打印结果
    for _, r := range results {
        fmt.Printf("%s:%d: %s\n", r.File, r.Line, r.Content)
    }

    elapsed := time.Since(startTime)
    fmt.Printf("\n搜索完成! 处理了 %d 个文件, 找到 %d 个匹配, 总耗时: %v\n",
        len(files), len(results), elapsed)
}

// 收集符合条件的文件
func collectFiles(rootDir string, config FilterConfig) []string {
    var files []string
    var mutex sync.Mutex
    var wg sync.WaitGroup

    // 创建通道来限制并发
    semaphore := make(chan struct{}, runtime.NumCPU())
    
    // 定义访问函数
    var walkFn filepath.WalkFunc = func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return nil // 忽略错误，继续处理
        }
        
        // 检查是否为目录
        if info.IsDir() {
            // 检查是否应该忽略此目录
            dirName := filepath.Base(path)
            for _, ignoreDir := range config.IgnoreDirs {
                if dirName == ignoreDir {
                    return filepath.SkipDir
                }
            }
            return nil
        }
        
        // 检查文件大小
        if info.Size() > config.MaxFileSize {
            return nil
        }
        
        // 检查文件扩展名
        ext := strings.ToLower(filepath.Ext(path))
        matched := false
        for _, validExt := range config.Extensions {
            if ext == validExt || "."+ext == validExt {
                matched = true
                break
            }
        }
        
        if !matched {
            return nil
        }
        
        // 使用工作池并发处理文件
        wg.Add(1)
        go func(filePath string) {
            defer wg.Done()
            
            // 获取信号量
            semaphore <- struct{}{}
            defer func() { <-semaphore }()
            
            mutex.Lock()
            files = append(files, filePath)
            mutex.Unlock()
        }(path)
        
        return nil
    }
    
    filepath.Walk(rootDir, walkFn)
    wg.Wait()
    
    return files
}

// 并行搜索文件
func searchFilesParallel(files []string, regex *regexp.Regexp, concurrency int) []Result {
    var results []Result
    resultChan := make(chan Result)
    done := make(chan struct{})
    
    // 启动收集结果的协程
    go func() {
        for r := range resultChan {
            results = append(results, r)
        }
        close(done)
    }()
    
    // 创建工作池
    var wg sync.WaitGroup
    fileChan := make(chan string, concurrency)
    
    // 启动工作协程
    for i := 0; i < concurrency; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            for file := range fileChan {
                searchFile(file, regex, resultChan)
            }
        }()
    }
    
    // 发送文件到工作池
    for _, file := range files {
        fileChan <- file
    }
    close(fileChan)
    
    // 等待所有工作完成
    wg.Wait()
    close(resultChan)
    <-done
    
    return results
}

// 在单个文件中搜索
func searchFile(file string, regex *regexp.Regexp, resultChan chan<- Result) {
    f, err := os.Open(file)
    if err != nil {
        return
    }
    defer f.Close()
    
    reader := bufio.NewReader(f)
    lineNum := 1
    
    for {
        line, err := reader.ReadString('\n')
        if err != nil {
            if err != io.EOF {
                return
            }
            if len(line) == 0 {
                break
            }
        }
        
        if regex.MatchString(line) {
            resultChan <- Result{
                File:    file,
                Line:    lineNum,
                Content: strings.TrimSuffix(line, "\n"),
            }
        }
        
        lineNum++
        if err == io.EOF {
            break
        }
    }
}