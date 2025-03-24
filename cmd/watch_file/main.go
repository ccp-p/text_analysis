package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// 配置参数
type Config struct {
    Directory  string        // 要监视的目录
    Extensions []string      // 要监视的文件扩展名
    Command    string        // 检测到变化时执行的命令
    Interval   time.Duration // 检查间隔
}

// 存储文件的修改时间信息
type FileInfo struct {
    Path    string
    ModTime time.Time
}

func main() {
    // 解析命令行参数
    dir := flag.String("dir", "D:\\download\\dest\\summary", "要监视的目录")
    exts := flag.String("exts", "js,jsx,ts,tsx,css,html", "要监视的文件扩展名(逗号分隔)")
    cmd := flag.String("cmd", "npm --version", "检测到变化时执行的命令")
    interval := flag.Duration("interval", 500*time.Millisecond, "检查间隔")
    flag.Parse()

    // 创建配置
    config := Config{
        Directory:  *dir,
        Extensions: strings.Split(*exts, ","),
        Command:    *cmd,
        Interval:   *interval,
    }

    // 验证目录存在
    if _, err := os.Stat(config.Directory); os.IsNotExist(err) {
        log.Fatalf("目录不存在: %s", config.Directory)
    }

    // 如果没有指定命令，报错
    // if config.Command == "" {
    //     log.Fatal("请使用 -cmd 参数指定检测到变化时要执行的命令")
    // }

    // 开始监视
    fmt.Printf("开始监视目录: %s\n", config.Directory)
    fmt.Printf("监视的文件类型: %s\n", strings.Join(config.Extensions, ", "))
    fmt.Printf("执行的命令: %s\n", config.Command)
    fmt.Printf("检查间隔: %v\n", config.Interval)
    fmt.Println("按 Ctrl+C 停止...")

    watchFiles(config)
}

func watchFiles(config Config) {
    // 存储上一次的文件信息
    lastFiles := make(map[string]FileInfo)

    // 初始扫描
    files := scanDirectory(config.Directory, config.Extensions)
    for path, info := range files {
        lastFiles[path] = info
    }

    // 定期扫描文件变化
    ticker := time.NewTicker(config.Interval)
    defer ticker.Stop()

    for range ticker.C {
        changed := false
        currentFiles := scanDirectory(config.Directory, config.Extensions)

        // 检查是否有文件被修改或添加
        for path, info := range currentFiles {
            last, exists := lastFiles[path]
            if !exists || last.ModTime != info.ModTime {
                fmt.Printf("检测到文件变化: %s\n", path)
                changed = true
                break
            }
        }

        // 检查是否有文件被删除
        for path := range lastFiles {
            if _, exists := currentFiles[path]; !exists {
                fmt.Printf("检测到文件被删除: %s\n", path)
                changed = true
                break
            }
        }

        // 如果有变化，执行命令
        if changed {
            fmt.Printf("执行命令: %s\n", config.Command)
            
            // 将命令拆分为命令和参数
            parts := strings.Fields(config.Command)
            if len(parts) == 0 {
                fmt.Println("无效的命令")
                continue
            }
            
            cmd := exec.Command(parts[0], parts[1:]...)
            cmd.Stdout = os.Stdout
            cmd.Stderr = os.Stderr
            
            err := cmd.Run()
            if err != nil {
                fmt.Printf("命令执行失败: %v\n", err)
            } else {
                fmt.Println("命令执行成功")
            }
            
            // 更新文件信息
            lastFiles = currentFiles
        }
    }
}

// 扫描目录中符合扩展名的所有文件
func scanDirectory(root string, extensions []string) map[string]FileInfo {
    files := make(map[string]FileInfo)

    // 遍历目录
    filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
        // 跳过错误和目录
        if err != nil || info.IsDir() {
            return nil
        }

        // 检查扩展名是否匹配
        ext := strings.ToLower(filepath.Ext(path))
        if ext != "" {
            ext = ext[1:] // 移除点号
            for _, validExt := range extensions {
                if ext == validExt {
                    files[path] = FileInfo{
                        Path:    path,
                        ModTime: info.ModTime(),
                    }
                    break
                }
            }
        }

        return nil
    })

    return files
}