package main

import (
    "bufio"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "sync"
    "time"
)

// ButtonData 结构体用于表示按钮数据
type ButtonData struct {
    Button      string // 按钮标识
    ProjectCode string // 项目代码
    Page        string // 页面路径
    ButtonValue string // 按钮值
    ButtonName  string // 页面上按钮的名称
    PageName    string // 页面名称
    LineNumber  int    // 行号(从1开始)
    SearchTime  time.Duration // 搜索耗时
    SourceFile  string // 找到按钮值的源文件
}

// 匹配结果的质量分级
const (
    MatchQualityHigh = iota + 3  // 高质量匹配（如包含addOperations的函数调用）
    MatchQualityMedium           // 中等质量匹配（如包含按钮关键词的函数调用）
    MatchQualityLow              // 低质量匹配（如简单的字符串匹配）
)

// 匹配结果结构体
type MatchResult struct {
    Line      string
    Quality   int
    FilePath  string
    ButtonName string // 页面上按钮的名称（函数注释或函数名）
}

// 正则表达式为全局变量，避免重复编译
var (
    // 注释匹配模式
    commentRegex = regexp.MustCompile(`^\s*//\s*(.+)`)
    
    // 函数定义模式
    functionDefRegex = regexp.MustCompile(`function\s+(\w+)\s*\(`)
    
    // 动态按钮构造模式
	
    dynamicButtonPatterns = []string{
		"_my_goDetailPage",
		"_myHistory_goDetailPage",
        "_moreVideoList_goDetailPage",
        "_toNoteImg",
        "_toSharecurrPage",
    }
    
    // 动态按钮与函数名映射
    buttonFunctionMap = map[string]string{
		"_my_goDetailPage": "goDetailPage",
		"_myHistory_goDetailPage": "goDetailPage",
        "_moreVideoList_goDetailPage": "goDetailPage",
        "_toNoteImg": "toNotePage",
        "_toSharecurrPage": "toSharecurrPage",
    }
    
    // 排除模式
    excludeRegex = regexp.MustCompile(`(?i)(^\s*</div|^\s*<!--)`)
)

func main() {
    // 记录程序开始时间
    startTime := time.Now()

    // 创建日志文件
    logFile, err := os.Create("button_search.log")
    if err != nil {
        fmt.Printf("创建日志文件失败: %v\n", err)
        return
    }
    defer logFile.Close()

    // 写入日志函数
    writeLog := func(format string, args ...interface{}) {
        message := fmt.Sprintf(format, args...)
        timestamp := time.Now().Format("2006-01-02 15:04:05")
        logLine := fmt.Sprintf("[%s] %s\n", timestamp, message)
        
        // 同时输出到控制台和日志文件
        fmt.Print(logLine)
        logFile.WriteString(logLine)
    }

    writeLog("程序开始执行")

    // 输入文件路径
    inputFile := "1.txt"
    
    // 默认项目目录，可以通过命令行参数覆盖
    projectDir := `D:\project\cx_project\china_mobile\gitProject\bigclass\src\main\webapp\res\wap\`
    if len(os.Args) > 1 {
        projectDir = os.Args[1]
    }
    
    writeLog("项目目录: %s", projectDir)
    writeLog("输入文件: %s", inputFile)
    
    // 打开文件
    file, err := os.Open(inputFile)
    if err != nil {
        writeLog("打开文件失败: %v", err)
        return
    }
    defer file.Close()
    
    // 读取并解析TSV文件
    buttonDataList, err := parseTsvFile(file)
    if err != nil {
        writeLog("解析文件失败: %v", err)
        return
    }
    
    writeLog("成功解析 %d 条按钮数据", len(buttonDataList))
    
    // 预先收集所有HTML和JS文件
    allFiles, err := collectAllFiles(projectDir)
    if err != nil {
        writeLog("收集文件失败: %v", err)
        return
    }
    
    writeLog("找到 %d 个HTML/JS文件用于搜索", len(allFiles))
    
    // 预先分析文件，提取函数定义和注释
    functionCommentMap := extractFunctionComments(allFiles, writeLog)
    writeLog("从文件中提取了 %d 个函数定义及其注释", len(functionCommentMap))
    
    // 使用并行处理加速搜索
    var wg sync.WaitGroup
    concurrency := 4 // 并发数
    dataChan := make(chan *ButtonData)
    
    writeLog("启动 %d 个并发工作协程", concurrency)
    
    // 启动工作协程
    for i := 0; i < concurrency; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            for data := range dataChan {
                buttonStartTime := time.Now()
                writeLog("[工作协程 %d] 开始搜索按钮: %s", id, data.Button)
                
                searchButtonValueInAllFiles(data, allFiles, functionCommentMap, writeLog)
                
                data.SearchTime = time.Since(buttonStartTime)
                writeLog("[工作协程 %d] 完成搜索按钮: %s, 耗时: %v, 找到: %v, 按钮名称: %s", 
                    id, data.Button, data.SearchTime, data.ButtonValue != "", data.ButtonName)
            }
        }(i)
    }
    
    // 发送任务
    go func() {
        for i := range buttonDataList {
            dataChan <- &buttonDataList[i]
        }
        close(dataChan)
    }()
    
    // 等待所有搜索完成
    wg.Wait()
    writeLog("所有按钮搜索完成")
    
    // 统计匹配结果
    var matchedCount, highQualityCount, withNameCount int
    for _, data := range buttonDataList {
        if data.ButtonValue != "" {
            matchedCount++
            if strings.Contains(data.ButtonValue, "addOpeartionsClickLog") || 
               strings.Contains(data.ButtonValue, "addOperationsClickLog") {
                highQualityCount++
            }
            if data.ButtonName != "" {
                withNameCount++
            }
        }
    }
    
    writeLog("匹配结果统计: 总计 %d 个按钮, 成功匹配 %d 个 (%.2f%%), 高质量匹配 %d 个 (%.2f%%), 有名称说明 %d 个 (%.2f%%)",
        len(buttonDataList), matchedCount, 
        float64(matchedCount)*100/float64(len(buttonDataList)),
        highQualityCount, 
        float64(highQualityCount)*100/float64(len(buttonDataList)),
        withNameCount,
        float64(withNameCount)*100/float64(len(buttonDataList)))
    
    // 创建输出文件
    outputFile := "result.txt"
    outFile, err := os.Create(outputFile)
    if err != nil {
        writeLog("创建输出文件失败: %v", err)
        return
    }
    defer outFile.Close()
    
    // 写入表头
    outFile.WriteString("button\tprojectcode\tpage\t按钮值\t页面上按钮的名称\t页面名称\t源文件\t搜索耗时(ms)\n")
    
    // 写入数据，保持TSV格式
    for _, data := range buttonDataList {
        line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
            data.Button,
            data.ProjectCode,
            data.Page,
            data.ButtonValue, // 这里可能是空字符串
            data.ButtonName,  // 这里是从注释或函数名中提取的按钮名称
            data.PageName,
            filepath.Base(data.SourceFile),
            data.SearchTime.Milliseconds())
        outFile.WriteString(line)
    }
    
    totalTime := time.Since(startTime)
    writeLog("程序执行完成，总耗时: %v, 结果保存到 %s", totalTime, outputFile)
    
    // 输出汇总结果
    fmt.Printf("\n======== 执行汇总 ========\n")
    fmt.Printf("总执行时间: %v\n", totalTime)
    fmt.Printf("处理按钮数: %d\n", len(buttonDataList))
    fmt.Printf("成功匹配数: %d (%.2f%%)\n", 
        matchedCount, float64(matchedCount)*100/float64(len(buttonDataList)))
    fmt.Printf("高质量匹配: %d (%.2f%%)\n", 
        highQualityCount, float64(highQualityCount)*100/float64(len(buttonDataList)))
    fmt.Printf("有名称说明: %d (%.2f%%)\n",
        withNameCount, float64(withNameCount)*100/float64(len(buttonDataList)))
    fmt.Printf("输出文件: %s\n", outputFile)
    fmt.Printf("日志文件: button_search.log\n")
}

// 预先提取所有函数及其注释
func extractFunctionComments(files []string, logFunc func(string, ...interface{})) map[string]string {
    functionCommentMap := make(map[string]string)
    
    for _, filePath := range files {
        // 跳过非JS文件
        if !strings.HasSuffix(strings.ToLower(filePath), ".js") {
            continue
        }
        
        file, err := os.Open(filePath)
        if err != nil {
            logFunc("打开文件失败: %s, 错误: %v", filePath, err)
            continue
        }
        
        scanner := bufio.NewScanner(file)
        var lastComment string
        
        // 逐行扫描文件
        for scanner.Scan() {
            line := scanner.Text()
            
            // 查找注释
            commentMatch := commentRegex.FindStringSubmatch(line)
            if len(commentMatch) > 1 {
                lastComment = commentMatch[1]
                continue
            }
            
            // 查找函数定义
            funcMatch := functionDefRegex.FindStringSubmatch(line)
            if len(funcMatch) > 1 {
                funcName := funcMatch[1]
                
                // 存储函数名和注释的映射
                if lastComment != "" {
                    functionCommentMap[funcName] = lastComment
                    logFunc("提取函数 %s 的注释: %s", funcName, lastComment)
                }
                
                // 重置注释，避免被下一个函数继承
                lastComment = ""
            }
        }
        
        file.Close()
    }
    
    return functionCommentMap
}

// 解析TSV文件
func parseTsvFile(file io.Reader) ([]ButtonData, error) {
    scanner := bufio.NewScanner(file)
    var buttonDataList []ButtonData
    lineNum := 0
    
    // 逐行读取文件
    for scanner.Scan() {
        lineNum++
        line := scanner.Text()
        
        // 跳过可能存在的注释行
        if strings.HasPrefix(line, "//") {
            continue
        }
        
        // 按制表符分割字段
        fields := strings.Split(line, "\t")
        
        // 检查是否是表头行
        if lineNum == 1 {
            continue
        }
        
        // 创建数据对象并安全地赋值
        data := ButtonData{
            LineNumber: lineNum,
        }
        
        if len(fields) > 0 {
            data.Button = strings.TrimSpace(fields[0])
        }
        
        if len(fields) > 1 {
            data.ProjectCode = strings.TrimSpace(fields[1])
        }
        
        if len(fields) > 2 {
            data.Page = strings.TrimSpace(fields[2])
        }
        
        if len(fields) > 3 {
            data.ButtonValue = strings.TrimSpace(fields[3])
        }
        
        if len(fields) > 4 {
            data.ButtonName = strings.TrimSpace(fields[4])
        }
        
        if len(fields) > 5 {
            data.PageName = strings.TrimSpace(fields[5])
        }
        
        // 添加到结果集，排除空行
        if data.Button != "" {
            buttonDataList = append(buttonDataList, data)
        }
    }
    
    return buttonDataList, scanner.Err()
}

// 预先收集所有HTML和JS文件
// 预先收集所有HTML和JS文件
func collectAllFiles(rootDir string) ([]string, error) {
    var files []string
    
    err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        
        if info.IsDir() {
            // 忽略 activityPages 和 node_modules 文件夹
            dirName := info.Name()
            if dirName == "activityPages" || dirName == "node_modules" || dirName ==".idea" {
                return filepath.SkipDir
            }
            return nil
        }
        
        ext := strings.ToLower(filepath.Ext(path))
        if ext == ".html" || ext == ".js" {
            files = append(files, path)
        }
        
        return nil
    })
    
    return files, err
}

// 在所有文件中查找按钮内容
func searchButtonValueInAllFiles(data *ButtonData, allFiles []string, functionCommentMap map[string]string, logFunc func(string, ...interface{})) {
    if data.Button == "" {
        return
    }
    
    // 分析按钮是否包含已知的动态模式后缀
    var dynamicSuffix, dynamicFuncName string
    for _, pattern := range dynamicButtonPatterns {
        if strings.HasSuffix(data.Button, pattern) {
            dynamicSuffix = pattern
            dynamicFuncName = buttonFunctionMap[pattern]
            break
        }
    }
    
    // 如果找到了动态模式，先尝试从函数定义中找名称
    if dynamicSuffix != "" && dynamicFuncName != "" {
        if comment, exists := functionCommentMap[dynamicFuncName]; exists {
            data.ButtonName = comment
            logFunc("按钮 '%s': 从函数定义中找到名称: %s", data.Button, comment)
        }
    }
    
    // 优先尝试先搜索与页面名称相关的文件
    pageFile := filepath.Base(data.Page)
    fileBase := strings.TrimSuffix(pageFile, filepath.Ext(pageFile))
    
    logFunc("按钮 '%s': 开始搜索, 相关页面: %s", data.Button, data.Page)
    
    // 先搜索可能性更高的文件（基于页面名称）
    relevantFiles := filterRelevantFiles(allFiles, fileBase)
    logFunc("按钮 '%s': 找到 %d 个相关文件", data.Button, len(relevantFiles))
    
    // 存储最佳匹配结果
    var bestMatch MatchResult
    
    // 首先在可能性高的文件中查找
    for _, filePath := range relevantFiles {
        match, err := searchButtonInFile(filePath, data.Button, dynamicSuffix, functionCommentMap)
        if err == nil && match.Line != "" {
            logFunc("按钮 '%s': 在文件 %s 中找到匹配, 质量级别: %d", 
                data.Button, filepath.Base(filePath), match.Quality)
            
            // 更新最佳匹配
            if match.Quality > bestMatch.Quality {
                bestMatch = match
                
                // 如果是高质量匹配，立即使用
                if match.Quality == MatchQualityHigh {
                    break
                }
            }
        }
    }
    
    // 如果在相关文件中未找到高质量匹配，则地毯式搜索所有文件
    if bestMatch.Quality < MatchQualityHigh {
        logFunc("按钮 '%s': 在相关文件中未找到高质量匹配，开始全局搜索", data.Button)
        
        for _, filePath := range allFiles {
            // 跳过已经搜索过的文件
            if contains(relevantFiles, filePath) {
                continue
            }
            
            match, err := searchButtonInFile(filePath, data.Button, dynamicSuffix, functionCommentMap)
            if err == nil && match.Line != "" {
                logFunc("按钮 '%s': 在文件 %s 中找到匹配, 质量级别: %d", 
                    data.Button, filepath.Base(filePath), match.Quality)
                
                // 更新最佳匹配
                if match.Quality > bestMatch.Quality {
                    bestMatch = match
                    
                    // 如果是高质量匹配，立即使用
                    if match.Quality == MatchQualityHigh {
                        break
                    }
                }
            }
        }
    }
    
    // 使用找到的最佳匹配
    if bestMatch.Line != "" {
        data.ButtonValue = bestMatch.Line
        data.SourceFile = bestMatch.FilePath
        
        // 优先使用从文件中找到的按钮名称
        if bestMatch.ButtonName != "" {
            data.ButtonName = bestMatch.ButtonName
        }
        
        logFunc("按钮 '%s': 最终使用匹配结果, 质量级别: %d, 源文件: %s, 按钮名称: %s", 
            data.Button, bestMatch.Quality, filepath.Base(bestMatch.FilePath), data.ButtonName)
    } else {
        data.ButtonValue = ""
        logFunc("按钮 '%s': 未找到任何匹配", data.Button)
    }
}

// 筛选与页面名称相关的文件（提高搜索效率）
func filterRelevantFiles(allFiles []string, baseName string) []string {
    var relevantFiles []string
    lowerBaseName := strings.ToLower(baseName)
    
    for _, file := range allFiles {
        fileName := strings.ToLower(filepath.Base(file))
        // 如果文件名包含页面基本名称，则优先考虑
        if strings.Contains(fileName, lowerBaseName) {
            relevantFiles = append(relevantFiles, file)
        }
    }
    
    return relevantFiles
}

// 检查slice是否包含字符串
func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}

// 在文件中搜索按钮内容，返回匹配质量与内容
func searchButtonInFile(filePath string, buttonText string, dynamicSuffix string, functionCommentMap map[string]string) (MatchResult, error) {
    // 空结果
    emptyResult := MatchResult{Quality: -1, FilePath: filePath}
    
    // 打开文件
    file, err := os.Open(filePath)
    if err != nil {
        return emptyResult, err
    }
    defer file.Close()
    
    scanner := bufio.NewScanner(file)
    
    // 为大行设置更大的buffer
    const maxScanTokenSize = 1024 * 1024
    buf := make([]byte, maxScanTokenSize)
    scanner.Buffer(buf, maxScanTokenSize)
    
    var lastComment string
    var inFunctionContext bool
    var currentFunction string
    
    // 基础按钮文本和动态按钮部分的匹配模式
    var baseButtonPattern, dynamicButtonPattern string
    if dynamicSuffix != "" {
        // 如果是动态按钮，构造两种模式：完整匹配和后缀匹配
        baseButtonPattern = regexp.QuoteMeta(buttonText)
        dynamicButtonPattern = regexp.QuoteMeta(dynamicSuffix)
    } else {
        // 普通按钮，只需匹配完整文本
        baseButtonPattern = regexp.QuoteMeta(buttonText)
    }
    
    // 高优先级匹配模式 (更可能是真实的按钮点击处理)
    highPriorityPatterns := []string{
        // addOpeartionsClickLog 模式
        `addOpeartionsClickLog\s*\(\s*\{\s*button\s*:\s*["']` + baseButtonPattern + `["']`,
        `addOpeartionsClickLog\s*\(\s*\{\s*button\s*:\s*[^}]*` + baseButtonPattern, // 动态构造的按钮
        `addOperationsClickLog\s*\(\s*\{\s*button\s*:\s*["']` + baseButtonPattern + `["']`,
        `addOperationsClickLog\s*\(\s*\{\s*button\s*:\s*[^}]*` + baseButtonPattern, // 动态构造的按钮
    }
    
    // 如果是动态按钮，添加特定后缀模式
    if dynamicSuffix != "" {
        highPriorityPatterns = append(highPriorityPatterns,
            `addOpeartionsClickLog\s*\(\s*\{\s*button\s*:\s*[^}]*` + dynamicButtonPattern,
            `addOperationsClickLog\s*\(\s*\{\s*button\s*:\s*[^}]*` + dynamicButtonPattern,
        )
    }
    
    // 中等优先级匹配模式 (可能是按钮相关，但不一定是点击处理)
    mediumPriorityPatterns := []string{
        // 作为事件处理函数中的参数
        `\(\s*["']` + baseButtonPattern + `["']\s*\)`,
        // 按钮定义模式
        `button\s*:\s*["']` + baseButtonPattern + `["']`,
        `button\s*:\s*[^,}]*` + baseButtonPattern, // 动态构造的按钮
        // 作为按钮ID或Class
        `id\s*=\s*["']` + baseButtonPattern + `["']`,
        `class\s*=\s*["'][^"']*` + baseButtonPattern + `[^"']*["']`,
    }
    
    // 低优先级匹配模式 (最宽泛的匹配)
    lowPriorityPatterns := []string{
        // 直接匹配
        baseButtonPattern,
        // 作为字符串
        `["']` + baseButtonPattern + `["']`,
    }
    
    // 组合所有正则表达式
    highPriorityRegex, err := regexp.Compile(`(?i)(` + strings.Join(highPriorityPatterns, "|") + `)`)
    if err != nil {
        return emptyResult, err
    }
    
    mediumPriorityRegex, err := regexp.Compile(`(?i)(` + strings.Join(mediumPriorityPatterns, "|") + `)`)
    if err != nil {
        return emptyResult, err
    }
    
    lowPriorityRegex, err := regexp.Compile(`(?i)(` + strings.Join(lowPriorityPatterns, "|") + `)`)
    if err != nil {
        return emptyResult, err
    }
    
    // 函数定义查找
    functionRegex := regexp.MustCompile(`function\s+(\w+)\s*\(`)
    
    // 逐行扫描文件查找最佳匹配
    bestMatch := emptyResult
    
    // 逐行扫描文件
    lineNum := 0
    for scanner.Scan() {
        lineNum++
        line := scanner.Text()
        
        // 清除前后空格
        cleanLine := strings.TrimSpace(line)
        
        // 跳过空行或明显的HTML结束标签和注释
        if cleanLine == "" || excludeRegex.MatchString(cleanLine) {
            continue
        }
        
        // 检查是否是注释行
        commentMatch := commentRegex.FindStringSubmatch(cleanLine)
        if len(commentMatch) > 1 {
            lastComment = commentMatch[1]
            continue
        }
        
        // 检查是否是函数定义开始
        funcMatch := functionRegex.FindStringSubmatch(cleanLine)
        if len(funcMatch) > 1 {
            currentFunction = funcMatch[1]
            inFunctionContext = true
            continue
        }
        
        // 按优先级依次检查
        if highPriorityRegex.MatchString(cleanLine) {
            // 高优先级匹配，尝试提取按钮名称
            buttonName := ""
            
            // 如果在函数内，使用函数名或注释作为按钮名称
            if inFunctionContext && currentFunction != "" {
                // 优先使用函数注释
                if comment, exists := functionCommentMap[currentFunction]; exists {
                    buttonName = comment
                } else if lastComment != "" {
                    // 或使用上一个注释
                    buttonName = lastComment
                } else {
                    // 最后使用函数名
                    buttonName = currentFunction
                }
            }
            
            // 截取过长的行
            if len(cleanLine) > 500 {
                cleanLine = cleanLine[:500] + "..."
            }
            
            return MatchResult{
                Line:      cleanLine,
                Quality:   MatchQualityHigh,
                FilePath:  filePath,
                ButtonName: buttonName,
            }, nil
        } else if mediumPriorityRegex.MatchString(cleanLine) {
            // 中优先级匹配，记录但继续搜索高优先级匹配
            if bestMatch.Quality < MatchQualityMedium {
                buttonName := ""
                
                // 同样尝试提取按钮名称
                if inFunctionContext && currentFunction != "" {
                    if comment, exists := functionCommentMap[currentFunction]; exists {
                        buttonName = comment
                    } else if lastComment != "" {
                        buttonName = lastComment
                    } else {
                        buttonName = currentFunction
                    }
                }
                
                if len(cleanLine) > 500 {
                    cleanLine = cleanLine[:500] + "..."
                }
                
                bestMatch = MatchResult{
                    Line:      cleanLine,
                    Quality:   MatchQualityMedium,
                    FilePath:  filePath,
                    ButtonName: buttonName,
                }
            }
        } else if lowPriorityRegex.MatchString(cleanLine) {
            // 低优先级匹配，仅当没有更好的匹配时使用
            if bestMatch.Quality < MatchQualityLow {
                buttonName := ""
                
                // 尝试提取按钮名称
                if inFunctionContext && currentFunction != "" {
                    if comment, exists := functionCommentMap[currentFunction]; exists {
                        buttonName = comment
                    } else if lastComment != "" {
                        buttonName = lastComment
                    } else {
                        buttonName = currentFunction
                    }
                }
                
                if len(cleanLine) > 500 {
                    cleanLine = cleanLine[:500] + "..."
                }
                
                bestMatch = MatchResult{
                    Line:      cleanLine,
                    Quality:   MatchQualityLow,
                    FilePath:  filePath,
                    ButtonName: buttonName,
                }
            }
        }
        
        // 检查是否是函数定义结束
        if inFunctionContext && cleanLine == "}" {
            inFunctionContext = false
            currentFunction = ""
            lastComment = ""
        }
    }
    
    if err := scanner.Err(); err != nil {
        return emptyResult, err
    }
    
    return bestMatch, nil
}