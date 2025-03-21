package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"textanalyzer/internal/analyzer"
	"textanalyzer/internal/finder"
	"textanalyzer/internal/pipeline"
)

// 定义命令行参数
var (
    directory    = flag.String("dir", "D:\\download\\dest", "目录路径")
    pattern      = flag.String("pattern", `\.txt$`, "文件名匹配模式(正则表达式)")
    topWords     = flag.Int("top", 20, "显示频率最高的词数量")
    summaryLines = flag.Int("summary", 5, "摘要句子数量")
    outputFile   = flag.String("out", "", "输出文件路径")
    verbose      = flag.Bool("v", false, "显示详细信息")
)

func main() {
    // 解析命令行参数
    flag.Parse()
    
    start := time.Now()
    
    // 初始化文件查找器
    fileFinder, err := finder.NewFileFinder(*pattern)
    if err != nil {
        fmt.Fprintf(os.Stderr, "错误: %v\n", err)
        os.Exit(1)
    }
    
    // 初始化分析器
    wordAnalyzer := analyzer.NewWordFrequencyAnalyzer()
    patternAnalyzer := analyzer.NewPatternAnalyzer()
    summaryGenerator := analyzer.NewSummaryGenerator()
    
    // 创建结果通道
    resultChan := make(chan []string)
    
    // 使用WaitGroup追踪goroutine
    var wg sync.WaitGroup
    
    // 处理函数链
    processor := func(line string) []string {
        // 词频分析
        wordAnalyzer.ProcessText(line)
        
        // 语法模式分析
        patternAnalyzer.ProcessText(line)
        
        // 摘要生成
        return summaryGenerator.ProcessText(line)
    }
    
    // 启动文件处理goroutines
    processedFiles := 0
    for filePath := range fileFinder.FindFiles(*directory) {
        if *verbose {
            fmt.Printf("处理文件: %s\n", filePath)
        }
        
        wg.Add(1)
        go pipeline.ProcessFile(filePath, processor, resultChan, &wg)
        processedFiles++
    }
    
    // 在另一个goroutine中等待所有处理完成，然后关闭结果通道
    go func() {
        wg.Wait()
        close(resultChan)
    }()
    
    // 处理结果
    linesProcessed := 0
    for range resultChan {
        // 这里我们只是计数
        linesProcessed++
    }
    
    // 生成报告
    report := map[string]interface{}{
        "文件分析统计": map[string]interface{}{
            "处理文件数":  processedFiles,
            "处理行数":   linesProcessed,
            "处理时间(秒)": time.Since(start).Seconds(),
        },
        "词频统计": map[string]interface{}{
            "词汇总数":      len(wordAnalyzer.GetWordFrequencies()),
            "最常见词汇(TOP": *topWords,
            "词频列表":      wordAnalyzer.GetTopWords(*topWords),
        },
        "语法模式分析": patternAnalyzer.GetPatternStatistics(),
        "文本摘要":   summaryGenerator.GenerateSummary(*summaryLines),
    }
    
    // 输出报告
    reportJson, _ := json.MarshalIndent(report, "", "  ")
    
    if *outputFile != "" {
        err := os.WriteFile(*outputFile, reportJson, 0644)
        if err != nil {
            fmt.Fprintf(os.Stderr, "保存报告失败: %v\n", err)
        } else {
            fmt.Printf("分析报告已保存到: %s\n", *outputFile)
        }
    } else {
        fmt.Println(string(reportJson))
    }
    
    fmt.Printf("\n处理完成! 耗时: %.2f秒\n", time.Since(start).Seconds())
}