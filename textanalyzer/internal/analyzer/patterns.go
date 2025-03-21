package analyzer

import (
    "regexp"
    "sync"
)

// PatternInfo 存储模式信息
type PatternInfo struct {
    Pattern     string
    Description string
    Count       int
    Examples    []string
}

// PatternAnalyzer 分析语法模式
type PatternAnalyzer struct {
    patterns map[string]*PatternInfo
    regexes  map[string]*regexp.Regexp
    mutex    sync.Mutex
}

// NewPatternAnalyzer 创建模式分析器
func NewPatternAnalyzer() *PatternAnalyzer {
    pa := &PatternAnalyzer{
        patterns: make(map[string]*PatternInfo),
        regexes:  make(map[string]*regexp.Regexp),
    }
    
    // 添加预定义的语法模式
    pa.AddPattern("question", `\w+\s+\w+\?`, "问句")
    pa.AddPattern("exclamation", `\w+\s+\w+!`, "感叹句")
    pa.AddPattern("quote", `"[^"]*"`, "引用")
    
    return pa
}

// AddPattern 添加待分析的模式
func (pa *PatternAnalyzer) AddPattern(name, pattern, description string) error {
    regex, err := regexp.Compile(pattern)
    if err != nil {
        return err
    }
    
    pa.mutex.Lock()
    defer pa.mutex.Unlock()
    
    pa.patterns[name] = &PatternInfo{
        Pattern:     pattern,
        Description: description,
        Count:       0,
        Examples:    make([]string, 0),
    }
    pa.regexes[name] = regex
    
    return nil
}

// ProcessText 识别文本中的语法模式
func (pa *PatternAnalyzer) ProcessText(text string) []string {
    matches := make([]string, 0)
    
    pa.mutex.Lock()
    defer pa.mutex.Unlock()
    
    // 针对每种模式进行匹配
    for name, regex := range pa.regexes {
        found := regex.FindAllString(text, -1)
        if len(found) > 0 {
            matches = append(matches, found...)
            
            // 更新统计信息
            info := pa.patterns[name]
            info.Count += len(found)
            
            // 保存示例(最多保存5个)
            for _, example := range found {
                if len(info.Examples) < 5 {
                    info.Examples = append(info.Examples, example)
                }
            }
        }
    }
    
    return matches
}

// GetPatternStatistics 获取模式统计信息
func (pa *PatternAnalyzer) GetPatternStatistics() map[string]*PatternInfo {
    pa.mutex.Lock()
    defer pa.mutex.Unlock()
    
    // 返回统计信息的副本
    result := make(map[string]*PatternInfo, len(pa.patterns))
    for name, info := range pa.patterns {
        // 复制值，而不仅仅是引用
        copiedExamples := make([]string, len(info.Examples))
        copy(copiedExamples, info.Examples)
        
        result[name] = &PatternInfo{
            Pattern:     info.Pattern,
            Description: info.Description,
            Count:       info.Count,
            Examples:    copiedExamples,
        }
    }
    
    return result
}