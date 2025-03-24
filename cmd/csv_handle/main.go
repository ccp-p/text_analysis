package main

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 数据行
type DataRow map[string]string

// 统计结果
type Stats struct {
    Min     float64
    Max     float64
    Sum     float64
    Count   int64
    Average float64
    Median  float64
}

// 数据处理配置
type ProcessConfig struct {
    InputFile  string
    OutputFile string
    Delimiter  string
    NumWorkers int
    GroupBy    string
    AggFields  []string
    SortBy     string
    SortDesc   bool
    FilterExpr string
    Limit      int
}

func main() {
    // 命令行参数
    inputFile := flag.String("input", "D:\\download\\dest\\summary\\彩讯股份个人电脑安全暨防钓鱼及敏感数据要求及宣贯（20240728）(1).xlsx", "输入CSV文件")
    outputFile := flag.String("output", "", "输出CSV文件")
    delimiter := flag.String("delimiter", ",", "字段分隔符")
    workers := flag.Int("workers", runtime.NumCPU(), "并发工作器数量")
    groupBy := flag.String("group", "", "分组字段")
    aggregate := flag.String("aggregate", "", "聚合计算的字段(逗号分隔)")
    sortBy := flag.String("sort", "", "排序字段")
    sortDesc := flag.Bool("desc", false, "降序排序")
    filterExpr := flag.String("filter", "", "过滤表达式")
    limit := flag.Int("limit", 0, "结果限制")
    showMemory := flag.Bool("memory", false, "显示内存使用情况")
    flag.Parse()

    if *inputFile == "" {
        fmt.Println("请指定输入文件，使用 -input 参数")
        flag.Usage()
        return
    }

    // 配置处理
    config := ProcessConfig{
        InputFile:  *inputFile,
        OutputFile: *outputFile,
        Delimiter:  *delimiter,
        NumWorkers: *workers,
        GroupBy:    *groupBy,
        SortBy:     *sortBy,
        SortDesc:   *sortDesc,
        FilterExpr: *filterExpr,
        Limit:      *limit,
    }

    if *aggregate != "" {
        config.AggFields = strings.Split(*aggregate, ",")
    }

    // 开始计时
    startTime := time.Now()

    // 打印初始信息
    fmt.Printf("开始处理文件: %s\n", *inputFile)
    fmt.Printf("并发工作器: %d\n", *workers)

    // 处理数据
    results, headers, err := processCSV(config)
    if err != nil {
        fmt.Printf("处理失败: %v\n", err)
        return
    }

    // 输出结果
    if *outputFile != "" {
        if err := writeResults(*outputFile, results, headers); err != nil {
            fmt.Printf("写入结果失败: %v\n", err)
        } else {
            fmt.Printf("结果已写入: %s\n", *outputFile)
        }
    } else {
        // 显示前几行
        displayLimit := 20
        if len(results) < displayLimit {
            displayLimit = len(results)
        }
        fmt.Printf("\n前 %d 行结果:\n", displayLimit)
        
        // 打印表头
        fmt.Println(strings.Join(headers, "\t"))
        fmt.Println(strings.Repeat("-", 80))
        
        // 打印数据行
        for i := 0; i < displayLimit; i++ {
            row := results[i]
            values := make([]string, 0, len(headers))
            for _, h := range headers {
                values = append(values, row[h])
            }
            fmt.Println(strings.Join(values, "\t"))
        }
        
        if len(results) > displayLimit {
            fmt.Printf("... 共 %d 行\n", len(results))
        }
    }

    // 报告执行时间
    elapsed := time.Since(startTime)
    fmt.Printf("\n处理完成，耗时: %v\n", elapsed)
    fmt.Printf("处理速度: %.2f 行/秒\n", float64(len(results))/elapsed.Seconds())

    // 显示内存使用
    if *showMemory {
        var m runtime.MemStats
        runtime.ReadMemStats(&m)
        fmt.Printf("内存使用: %.2f MB\n", float64(m.Alloc)/1024/1024)
    }
}

// 处理CSV文件
func processCSV(config ProcessConfig) ([]DataRow, []string, error) {
    // 打开输入文件
    file, err := os.Open(config.InputFile)
    if err != nil {
        return nil, nil, fmt.Errorf("无法打开文件: %v", err)
    }
    defer file.Close()

    // 创建CSV读取器
    reader := csv.NewReader(file)
    reader.Comma = []rune(config.Delimiter)[0]
    
    // 读取表头
    headers, err := reader.Read()
    if err != nil {
        return nil, nil, fmt.Errorf("读取表头失败: %v", err)
    }
    
    // 估计文件大小和行数
    fileInfo, err := file.Stat()
    if err != nil {
        return nil, nil, fmt.Errorf("获取文件信息失败: %v", err)
    }
    
    fileSize := fileInfo.Size()
    estimatedRows := estimateRowCount(file, fileSize)
    fmt.Printf("估计数据行数: 约 %d 行\n", estimatedRows)
    
    // 重置文件指针
    file.Seek(0, 0)
    reader = csv.NewReader(file)
    reader.Comma = []rune(config.Delimiter)[0]
    _, _ = reader.Read() // 跳过表头
    
    // 创建工作池
    rows := make(chan DataRow, 10000)
    results := make([]DataRow, 0, estimatedRows)
    var wg sync.WaitGroup
    
    // 启动工作协程
    for i := 0; i < config.NumWorkers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            for row := range rows {
                // 应用过滤
                if config.FilterExpr != "" && !applyFilter(row, config.FilterExpr) {
                    continue
                }
                
                // 处理数据行
                processRow(row, config.AggFields)
            }
        }()
    }
    
    // 读取和分配行
    go func() {
        scanner := bufio.NewScanner(file)
        // 跳过已读的表头
        scanner.Scan()
        
        lineCount := 0
        for scanner.Scan() {
            lineCount++
            line := scanner.Text()
            fields := strings.Split(line, config.Delimiter)
            
            if len(fields) != len(headers) {
                continue // 跳过字段数不匹配的行
            }
            
            // 创建数据行
            row := make(DataRow)
            for i, header := range headers {
                row[header] = fields[i]
            }
            
            rows <- row
            
            // 每处理10万行打印一次进度
            if lineCount%100000 == 0 {
                fmt.Printf("已处理 %d 行...\n", lineCount)
            }
        }
        
        close(rows)
        fmt.Printf("共读取 %d 行数据\n", lineCount)
    }()
    
    // 等待处理完成
    wg.Wait()
    
    // 处理分组和聚合
    if config.GroupBy != "" {
        results = groupAndAggregate(rows, config.GroupBy, config.AggFields)
    } else {
        // 将所有行收集到结果集
        for row := range rows {
            results = append(results, row)
        }
    }
    
    // 排序结果
    if config.SortBy != "" {
        sortResults(results, config.SortBy, config.SortDesc)
    }
    
    // 限制结果数量
    if config.Limit > 0 && len(results) > config.Limit {
        results = results[:config.Limit]
    }
    
    return results, headers, nil
}

// 估计文件的行数
func estimateRowCount(file *os.File, fileSize int64) int {
    // 读取前10000个字节来估计每行的平均大小
    buffer := make([]byte, min(10000, fileSize))
    file.Seek(0, 0)
    n, _ := file.Read(buffer)
    
    // 计算换行符的数量
    newlines := 0
    for i := 0; i < n; i++ {
        if buffer[i] == '\n' {
            newlines++
        }
    }
    
    // 如果没有找到换行符，假定每行100个字节
    if newlines == 0 {
        return int(fileSize / 100)
    }
    
    // 计算平均行大小并估计总行数
    avgLineSize := float64(n) / float64(newlines)
    return int(float64(fileSize) / avgLineSize)
}

// 处理数据行
func processRow(row DataRow, aggFields []string) {
    // 对数值字段进行转换
    for _, field := range aggFields {
        if val, ok := row[field]; ok {
            // 尝试将字符串转换为数值，以便后续聚合计算
            if num, err := strconv.ParseFloat(val, 64); err == nil {
                // 将处理后的值存回行中
                row[field] = fmt.Sprintf("%.2f", num)
            }
        }
    }
}

// 应用过滤条件
func applyFilter(row DataRow, filterExpr string) bool {
    // 简单的过滤表达式解析和应用
    // 格式: field=value 或 field>value 等
    parts := strings.SplitN(filterExpr, "=", 2)
    if len(parts) != 2 {
        return true // 无效表达式，不过滤
    }
    
    field := strings.TrimSpace(parts[0])
    value := strings.TrimSpace(parts[1])
    
    if val, ok := row[field]; ok {
        return val == value
    }
    
    return false
}

// 分组和聚合
func groupAndAggregate(rows chan DataRow, groupBy string, aggFields []string) []DataRow {
    groups := make(map[string][]DataRow)
    
    // 按分组字段收集行
    for row := range rows {
        groupValue := row[groupBy]
        if _, ok := groups[groupValue]; !ok {
            groups[groupValue] = make([]DataRow, 0, 100)
        }
        groups[groupValue] = append(groups[groupValue], row)
    }
    
    // 对每个分组执行聚合计算
    results := make([]DataRow, 0, len(groups))
    for groupValue, groupRows := range groups {
        aggregated := make(DataRow)
        aggregated[groupBy] = groupValue
        
        // 对每个聚合字段计算统计
        for _, field := range aggFields {
            stats := calculateStats(groupRows, field)
            aggregated[field+"_min"] = fmt.Sprintf("%.2f", stats.Min)
            aggregated[field+"_max"] = fmt.Sprintf("%.2f", stats.Max)
            aggregated[field+"_avg"] = fmt.Sprintf("%.2f", stats.Average)
            aggregated[field+"_sum"] = fmt.Sprintf("%.2f", stats.Sum)
            aggregated[field+"_count"] = fmt.Sprintf("%d", stats.Count)
            aggregated[field+"_median"] = fmt.Sprintf("%.2f", stats.Median)
        }
        
        results = append(results, aggregated)
    }
    
    return results
}

// 计算统计值
func calculateStats(rows []DataRow, field string) Stats {
    var values []float64
    var sum float64
    var count int64
    min := math.MaxFloat64
    max := -math.MaxFloat64
    
    // 收集所有值
    for _, row := range rows {
        if val, ok := row[field]; ok {
            if num, err := strconv.ParseFloat(val, 64); err == nil {
                values = append(values, num)
                sum += num
                count++
                
                if num < min {
                    min = num
                }
                if num > max {
                    max = num
                }
            }
        }
    }
    
    // 如果没有有效值，返回默认值
    if count == 0 {
        return Stats{Min: 0, Max: 0, Sum: 0, Count: 0, Average: 0, Median: 0}
    }
    
    // 计算平均值
    avg := sum / float64(count)
    
    // 计算中位数
    sort.Float64s(values)
    var median float64
    middle := len(values) / 2
    if len(values)%2 == 0 {
        median = (values[middle-1] + values[middle]) / 2
    } else {
        median = values[middle]
    }
    
    return Stats{
        Min:     min,
        Max:     max,
        Sum:     sum,
        Count:   count,
        Average: avg,
        Median:  median,
    }
}

// 对结果进行排序
func sortResults(results []DataRow, sortBy string, descending bool) {
    sort.Slice(results, func(i, j int) bool {
        // 尝试作为数值比较
        a, aErr := strconv.ParseFloat(results[i][sortBy], 64)
        b, bErr := strconv.ParseFloat(results[j][sortBy], 64)
        
        if aErr == nil && bErr == nil {
            if descending {
                return a > b
            }
            return a < b
        }
        
        // 如果不是数值，按字符串比较
        if descending {
            return results[i][sortBy] > results[j][sortBy]
        }
        return results[i][sortBy] < results[j][sortBy]
    })
}

// 写入结果到输出文件
func writeResults(outputFile string, results []DataRow, headers []string) error {
    // 创建输出目录
    outputDir := filepath.Dir(outputFile)
    if outputDir != "." {
        if err := os.MkdirAll(outputDir, 0755); err != nil {
            return fmt.Errorf("创建输出目录失败: %v", err)
        }
    }
    
    // 创建输出文件
    file, err := os.Create(outputFile)
    if err != nil {
        return fmt.Errorf("创建输出文件失败: %v", err)
    }
    defer file.Close()
    
    writer := csv.NewWriter(file)
    defer writer.Flush()
    
    // 写入表头
    if err := writer.Write(headers); err != nil {
        return fmt.Errorf("写入表头失败: %v", err)
    }
    
    // 写入数据行
    for _, row := range results {
        values := make([]string, 0, len(headers))
        for _, header := range headers {
            values = append(values, row[header])
        }
        if err := writer.Write(values); err != nil {
            return fmt.Errorf("写入数据行失败: %v", err)
        }
    }
    
    return nil
}

// Utility functions
func min(a, b int64) int64 {
    if a < b {
        return a
    }
    return b
}