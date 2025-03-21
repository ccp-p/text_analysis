package finder

import (
	"os"
	"path/filepath"
	"regexp"
)

// FileFinder 查找匹配模式的文件
type FileFinder struct {
    pattern *regexp.Regexp
}

// NewFileFinder 创建文件查找器
func NewFileFinder(pattern string) (*FileFinder, error) {
    regex, err := regexp.Compile(pattern)
    if err != nil {
        return nil, err
    }
    return &FileFinder{pattern: regex}, nil
}

// FindFiles 查找目录中匹配模式的文件
func (f *FileFinder) FindFiles(directory string) <-chan string {
    fileChannel := make(chan string)
    
    go func() {
        defer close(fileChannel)
        
        // 遍历目录中的所有文件
        err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
            // 处理错误
            if err != nil {
                return err
            }
            
            // 跳过目录
            if info.IsDir() {
                return nil
            }
            
            // 检查是否匹配模式
            if f.pattern.MatchString(info.Name()) {
                fileChannel <- path
            }
            
            return nil
        })
        
        if err != nil {
            // 处理错误，可以发送到错误通道
            // 简化示例中省略
        }
    }()
    
    return fileChannel
}