package analyzer

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// WordFrequencyAnalyzer 分析词频
type WordFrequencyAnalyzer struct {
    wordRegex *regexp.Regexp
    freqMap   map[string]int
    mutex     sync.Mutex
}

// NewWordFrequencyAnalyzer 创建词频分析器
func NewWordFrequencyAnalyzer() *WordFrequencyAnalyzer {
    return &WordFrequencyAnalyzer{
        wordRegex: regexp.MustCompile(`\w+`),
        freqMap:   make(map[string]int),
    }
}

// ProcessText 处理文本并更新词频
func (wfa *WordFrequencyAnalyzer) ProcessText(text string) []string {
    // 转为小写
    text = strings.ToLower(text)
    
    // 找出所有单词
    words := wfa.wordRegex.FindAllString(text, -1)
    
    // 更新词频map
    wfa.mutex.Lock()
    for _, word := range words {
        wfa.freqMap[word]++
    }
    wfa.mutex.Unlock()
    
    return words
}

// GetTopWords 获取出现频率最高的单词
func (wfa *WordFrequencyAnalyzer) GetTopWords(n int) map[string]int {
    result := make(map[string]int)
    
    // 创建词频切片
    type wordFreq struct {
        word string
        freq int
    }
    
    wfa.mutex.Lock()
    defer wfa.mutex.Unlock()
    
    wordFreqs := make([]wordFreq, 0, len(wfa.freqMap))
    for word, freq := range wfa.freqMap {
        wordFreqs = append(wordFreqs, wordFreq{word, freq})
    }
    
    // 按频率排序
    sort.Slice(wordFreqs, func(i, j int) bool {
        return wordFreqs[i].freq > wordFreqs[j].freq
    })
    
    // 获取前n个
    count := 0
    for _, wf := range wordFreqs {
        if count >= n {
            break
        }
        result[wf.word] = wf.freq
        count++
    }
    
    return result
}

// GetWordFrequencies 获取所有词频
func (wfa *WordFrequencyAnalyzer) GetWordFrequencies() map[string]int {
    wfa.mutex.Lock()
    defer wfa.mutex.Unlock()
    
    // 返回词频map的副本
    result := make(map[string]int, len(wfa.freqMap))
    for word, freq := range wfa.freqMap {
        result[word] = freq
    }
    
    return result
}