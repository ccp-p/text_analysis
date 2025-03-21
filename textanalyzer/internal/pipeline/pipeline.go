package pipeline

import (
	"bufio"
	"os"
	"sync"
)

// TextProcessor 定义文本处理函数类型
type TextProcessor func(string) []string

// TextLineProcessor 按行处理文本
func ProcessFile(filePath string, processor TextProcessor, resultChan chan<- []string, wg *sync.WaitGroup) {
    defer wg.Done()
    
    // 打开文件
    file, err := os.Open(filePath)
    if err != nil {
        // 处理错误
        return
    }
    defer file.Close()
    
    // 创建扫描器
    scanner := bufio.NewScanner(file)
    scanner.Split(bufio.ScanLines)
    
    // 处理每一行
    for scanner.Scan() {
        line := scanner.Text()
        // 处理该行文本
        results := processor(line)
        resultChan <- results
    }
}

// 构建处理函数链
func ComposeProcessors(processors ...func(string) string) TextProcessor {
    return func(input string) []string {
        result := input
        var results []string
        
        // 应用所有处理器
        for _, proc := range processors {
            result = proc(result)
            results = append(results, result)
        }
        
        return results
    }
}