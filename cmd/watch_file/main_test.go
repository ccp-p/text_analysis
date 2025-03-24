package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 测试扫描目录功能
func TestScanDirectory(t *testing.T) {
    // 创建临时测试目录
    tempDir, err := ioutil.TempDir("", "watch_file_test")
    if err != nil {
        t.Fatalf("创建临时目录失败: %v", err)
    }
    defer os.RemoveAll(tempDir) // 测试结束后清理

    // 创建测试文件
    testFiles := map[string]string{
        "test1.js":   "console.log('test1');",
        "test2.css":  "body { color: red; }",
        "test3.html": "<html><body>Test</body></html>",
        "test4.txt":  "This is a text file", // 不在扩展名列表中
        "test5.jsx":  "const Component = () => <div>Hello</div>",
    }

    for name, content := range testFiles {
        filePath := filepath.Join(tempDir, name)
        err := ioutil.WriteFile(filePath, []byte(content), 0644)
        if err != nil {
            t.Fatalf("创建测试文件失败 %s: %v", name, err)
        }
    }

    // 定义要监视的扩展名
    extensions := []string{"js", "css", "html", "jsx"}

    // 运行扫描目录函数
    files := scanDirectory(tempDir, extensions)

    // 验证结果
    if len(files) != 4 { // 应该有4个匹配的文件
        t.Errorf("应该找到4个文件，但实际找到了 %d 个", len(files))
    }

    // 检查是否找到了所有正确扩展名的文件
    expectedFiles := []string{"test1.js", "test2.css", "test3.html", "test5.jsx"}
    for _, name := range expectedFiles {
        filePath := filepath.Join(tempDir, name)
        if _, ok := files[filePath]; !ok {
            t.Errorf("没有找到应该匹配的文件: %s", name)
        }
    }

    // 检查是否过滤掉了不匹配的扩展名
    txtFilePath := filepath.Join(tempDir, "test4.txt")
    if _, ok := files[txtFilePath]; ok {
        t.Errorf("文件 test4.txt 不应该被包含在结果中")
    }
}

// 测试文件变更检测逻辑
func TestFileChangeDetection(t *testing.T) {
    // 创建临时测试目录
    tempDir, err := ioutil.TempDir("", "watch_file_change_test")
    if err != nil {
        t.Fatalf("创建临时目录失败: %v", err)
    }
    defer os.RemoveAll(tempDir)

    // 创建初始测试文件
    testFile := filepath.Join(tempDir, "test.js")
    err = ioutil.WriteFile(testFile, []byte("initial content"), 0644)
    if err != nil {
        t.Fatalf("创建测试文件失败: %v", err)
    }

    // 定义要监视的扩展名
    extensions := []string{"js"}

    // 获取初始文件状态
    initialFiles := scanDirectory(tempDir, extensions)
    if len(initialFiles) != 1 {
        t.Fatalf("应该找到1个文件，但实际找到了 %d 个", len(initialFiles))
    }

    // 确保足够的时间差以检测修改
    time.Sleep(1 * time.Second)

    // 修改文件
    err = ioutil.WriteFile(testFile, []byte("updated content"), 0644)
    if err != nil {
        t.Fatalf("更新测试文件失败: %v", err)
    }

    // 获取更新后的文件状态
    updatedFiles := scanDirectory(tempDir, extensions)

    // 检查文件修改时间是否变化
    initialModTime := initialFiles[testFile].ModTime
    updatedModTime := updatedFiles[testFile].ModTime
    
    if !updatedModTime.After(initialModTime) {
        t.Errorf("更新后的文件修改时间应该晚于初始时间")
    }
}

// 测试配置创建
func TestConfigCreation(t *testing.T) {
    // 测试配置创建
    config := Config{
        Directory:  "/path/to/dir",
        Extensions: []string{"js", "css"},
        Command:    "echo test",
        Interval:   500 * time.Millisecond,
    }

    // 验证配置字段
    if config.Directory != "/path/to/dir" {
        t.Errorf("配置目录不匹配，期望 /path/to/dir，得到 %s", config.Directory)
    }
    
    if len(config.Extensions) != 2 || config.Extensions[0] != "js" || config.Extensions[1] != "css" {
        t.Errorf("配置扩展名不匹配，期望 [js css]，得到 %v", config.Extensions)
    }
    
    if config.Command != "echo test" {
        t.Errorf("配置命令不匹配，期望 'echo test'，得到 %s", config.Command)
    }
    
    if config.Interval != 500*time.Millisecond {
        t.Errorf("配置间隔不匹配，期望 500ms，得到 %v", config.Interval)
    }
}

// 模拟命令执行功能测试
// 注意: 这个测试不会实际运行命令，而是验证命令解析逻辑
func TestCommandParsing(t *testing.T) {
    testCases := []struct {
        name          string
        command       string
        expectedParts []string
        shouldError   bool
    }{
        {
            name:          "简单命令",
            command:       "echo hello",
            expectedParts: []string{"echo", "hello"},
            shouldError:   false,
        },
        {
            name:          "多参数命令",
            command:       "go build -o app.exe main.go",
            expectedParts: []string{"go", "build", "-o", "app.exe", "main.go"},
            shouldError:   false,
        },
        {
            name:          "空命令",
            command:       "",
            expectedParts: []string{},
            shouldError:   true,
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            parts := strings.Fields(tc.command)
            
            if len(parts) == 0 && !tc.shouldError {
                t.Errorf("命令 '%s' 应该解析成功", tc.command)
            }
            
            if len(parts) > 0 && tc.shouldError {
                t.Errorf("命令 '%s' 应该解析失败", tc.command)
            }
            
            if len(parts) != len(tc.expectedParts) {
                t.Errorf("解析结果长度不匹配，期望 %d，得到 %d", len(tc.expectedParts), len(parts))
                return
            }
            
            for i, part := range parts {
                if part != tc.expectedParts[i] {
                    t.Errorf("参数 #%d 不匹配，期望 '%s'，得到 '%s'", i, tc.expectedParts[i], part)
                }
            }
        })
    }
}