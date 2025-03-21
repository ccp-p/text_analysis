package analyzer

import (
    "sort"
    "strings"
    "sync"
)

// SummaryGenerator 生成文本摘要
type SummaryGenerator struct {
    sentences       []string
    sentenceScores  map[string]float64
    wordFrequencies map[string]int
    mutex           sync.Mutex
}

// NewSummaryGenerator 创建摘要生成器
func NewSummaryGenerator() *SummaryGenerator {
    return &SummaryGenerator{
        sentences:       make([]string, 0),
        sentenceScores:  make(map[string]float64),
        wordFrequencies: make(map[string]int),
    }
}

// ProcessText 处理文本，积累句子
func (sg *SummaryGenerator) ProcessText(text string) []string {
    // 分割成句子
    sentences := splitIntoSentences(text)
    
    sg.mutex.Lock()
    defer sg.mutex.Unlock()
    
    // 添加到句子集合中
    sg.sentences = append(sg.sentences, sentences...)
    
    // 更新词频
    for _, sentence := range sentences {
        words := strings.Fields(strings.ToLower(sentence))
        for _, word := range words {
            sg.wordFrequencies[word]++
        }
    }
    
    return sentences
}

// 将文本分割成句子
func splitIntoSentences(text string) []string {
    // 简化版本，仅按 . ! ? 分割
    text = strings.ReplaceAll(text, ".", ".|")
    text = strings.ReplaceAll(text, "!", "!|")
    text = strings.ReplaceAll(text, "?", "?|")
    
    sentences := strings.Split(text, "|")
    var result []string
    
    for _, s := range sentences {
        s = strings.TrimSpace(s)
        if s != "" {
            result = append(result, s)
        }
    }
    
    return result
}

// GenerateSummary 生成文本摘要
func (sg *SummaryGenerator) GenerateSummary(numSentences int) []string {
    sg.mutex.Lock()
    defer sg.mutex.Unlock()
    
    // 计算句子得分
    for _, sentence := range sg.sentences {
        words := strings.Fields(strings.ToLower(sentence))
        score := 0.0
        
        for _, word := range words {
            score += float64(sg.wordFrequencies[word])
        }
        
        // 标准化得分(按句子长度)
        if len(words) > 0 {
            score /= float64(len(words))
        }
        
        sg.sentenceScores[sentence] = score
    }
    
    // 按分数排序句子
    type scoredSentence struct {
        sentence string
        score    float64
    }
    
    var scoredSentences []scoredSentence
    for sentence, score := range sg.sentenceScores {
        scoredSentences = append(scoredSentences, scoredSentence{sentence, score})
    }
    
    sort.Slice(scoredSentences, func(i, j int) bool {
        return scoredSentences[i].score > scoredSentences[j].score
    })
    
    // 选择分数最高的句子
    var summary []string
    for i, ss := range scoredSentences {
        if i >= numSentences {
            break
        }
        summary = append(summary, ss.sentence)
    }
    
    return summary
}