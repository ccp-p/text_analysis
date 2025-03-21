package analyzer

import (
	"bufio"
	"os"
	"regexp"
	"strings"
	"sync"
)

// SearchReplacer 执行搜索和替换
type SearchReplacer struct {
    searchPattern  *regexp.Regexp
    replacement    string
    matchCount     int
    replaceCount   int
    matchContexts  []string
    mutex          sync.Mutex
    maxContexts    int
}

// NewSearchReplacer 创建搜索替换器
func NewSearchReplacer(searchPattern, replacement string, maxContexts int) (*SearchReplacer, error) {
    regex, err := regexp.Compile(searchPattern)
    if err != nil {
        return nil, err
    }
    
    return &SearchReplacer{
        searchPattern: regex,
        replacement:   replacement,
        matchContexts: make([]string, 0),
        maxContexts:   maxContexts,
    }, nil
}

// Search 在文本中搜索
func (sr *SearchReplacer) Search(text string) bool {
    matches := sr.searchPattern.FindAllStringIndex(text, -1)
    
    if len(matches) > 0 {
        sr.mutex.Lock()
        defer sr.mutex.Unlock()
        
        sr.matchCount += len(matches)
        
        // 保存上下文
        if len(sr.matchContexts) < sr.maxContexts {
            // 提取匹配上下文
            for _, match := range matches {
                start := match[0]
                end := match[1]
                
                // 获取前后文
                contextStart := start - 20
                if contextStart < 0 {
                    contextStart = 0
                }
                
                contextEnd := end + 20
                if contextEnd > len(text) {
                    contextEnd = len(text)
                }
                
                context := text[contextStart:start] + "【" + text[start:end] + "】" + text[end:contextEnd]
                sr.matchContexts = append(sr.matchContexts, context)
                
                if len(sr.matchContexts) >= sr.maxContexts {
                    break
                }
            }
        }
        
        return true
    }
    
    return false
}

// Replace 执行替换
func (sr *SearchReplacer) Replace(text string) string {
    result := sr.searchPattern.ReplaceAllString(text, sr.replacement)
    
    replacements := 0
    if result != text {
        replacements = strings.Count(text, sr.searchPattern.String()) - strings.Count(result, sr.searchPattern.String())
        
        sr.mutex.Lock()
        sr.replaceCount += replacements
        sr.mutex.Unlock()
    }
    
    return result
}

// SearchAndReplace 同时执行搜索和替换
func (sr *SearchReplacer) SearchAndReplace(text string) (string, bool) {
    found := sr.Search(text)
    result := sr.Replace(text)
    return result, found
}

// SearchAndReplaceFile 对文件执行搜索替换
func (sr *SearchReplacer) SearchAndReplaceFile(inputFile, outputFile string) error {
    // 打开输入文件
    in, err := os.Open(inputFile)
    if err != nil {
        return err
    }
    defer in.Close()
    
    // 创建输出文件
    out, err := os.Create(outputFile)
    if err != nil {
        return err
    }
    defer out.Close()
    
    // 逐行处理
    scanner := bufio.NewScanner(in)
    writer := bufio.NewWriter(out)
    defer writer.Flush()
    
    for scanner.Scan() {
        line := scanner.Text()
        replacedLine, _ := sr.SearchAndReplace(line)
        
        _, err := writer.WriteString(replacedLine + "\n")
        if err != nil {
            return err
        }
    }
    
    return scanner.Err()
}

// GetStatistics 获取统计信息
func (sr *SearchReplacer) GetStatistics() map[string]interface{} {
    sr.mutex.Lock()
    defer sr.mutex.Unlock()
    
    stats := map[string]interface{}{
        "搜索模式":   sr.searchPattern.String(),
        "替换文本":   sr.replacement,
        "匹配总数":   sr.matchCount,
        "替换总数":   sr.replaceCount,
        "匹配上下文样例": sr.matchContexts,
    }
    
    return stats
}