package main

import (
    "bufio"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"
    "sync"
    "time"

    "github.com/fatih/color"
)

// 配置结构
type Config struct {
    Name        string            `json:"name"`
    Scripts     map[string]string `json:"scripts"`
    WatchDirs   []string          `json:"watchDirs"`
    WatchExts   []string          `json:"watchExts"`
    Environment map[string]string `json:"env"`
}

// 彩色输出
var (
    infoColor    = color.New(color.FgCyan).SprintFunc()
    successColor = color.New(color.FgGreen).SprintFunc()
    errorColor   = color.New(color.FgRed).SprintFunc()
    warnColor    = color.New(color.FgYellow).SprintFunc()
)

// 全局变量
var (
    configPath string
    config     Config
    wg         sync.WaitGroup
    processes  = make(map[string]*exec.Cmd)
    procMutex  sync.Mutex
)

func main() {
    // 解析命令行参数
    flag.StringVar(&configPath, "config", "devtool.json", "配置文件路径")
    flag.Parse()

    // 加载配置
    if err := loadConfig(); err != nil {
        fmt.Printf("%s 加载配置失败: %v\n", errorColor("错误"), err)
        os.Exit(1)
    }

    fmt.Printf("%s 项目: %s\n", infoColor("信息"), config.Name)
    fmt.Printf("%s 可用的命令:\n", infoColor("信息"))
    for name := range config.Scripts {
        fmt.Printf("  - %s\n", name)
    }

    // 如果指定了命令参数，直接执行
    if flag.NArg() > 0 {
        scriptName := flag.Arg(0)
        args := flag.Args()[1:]
        runScript(scriptName, args)
        return
    }

    // 否则，进入交互模式
    interactiveMode()
}

// 加载配置文件
func loadConfig() error {
    file, err := os.Open(configPath)
    if err != nil {
        // 如果配置文件不存在，创建默认配置
        if os.IsNotExist(err) {
            config = Config{
                Name: filepath.Base(getCurrentDir()),
                Scripts: map[string]string{
                    "start":  "echo '请在配置文件中添加启动脚本'",
                    "build":  "echo '请在配置文件中添加构建脚本'",
                    "test":   "echo '请在配置文件中添加测试脚本'",
                    "watch":  "echo '请在配置文件中添加监视脚本'",
                    "format": "echo '请在配置文件中添加格式化脚本'",
                },
                WatchDirs: []string{"src", "public"},
                WatchExts: []string{"js", "jsx", "ts", "tsx", "css", "scss", "html"},
                Environment: map[string]string{
                    "NODE_ENV": "development",
                },
            }
            return saveConfig()
        }
        return err
    }
    defer file.Close()

    return json.NewDecoder(file).Decode(&config)
}

// 保存配置文件
func saveConfig() error {
    file, err := os.Create(configPath)
    if err != nil {
        return err
    }
    defer file.Close()

    encoder := json.NewEncoder(file)
    encoder.SetIndent("", "  ")
    return encoder.Encode(config)
}

// 获取当前目录
func getCurrentDir() string {
    dir, err := os.Getwd()
    if err != nil {
        return "my-project"
    }
    return dir
}

// 交互模式
func interactiveMode() {
    reader := bufio.NewReader(os.Stdin)

    for {
        fmt.Printf("\n%s 请输入命令 (help 查看帮助): ", infoColor(">"))
        input, err := reader.ReadString('\n')
        if err != nil {
            fmt.Printf("%s 读取输入失败: %v\n", errorColor("错误"), err)
            continue
        }

        input = strings.TrimSpace(input)
        parts := strings.Fields(input)
        if len(parts) == 0 {
            continue
        }

        command := parts[0]
        args := parts[1:]

        switch command {
        case "exit", "quit":
            cleanupProcesses()
            fmt.Println(successColor("再见!"))
            return
        case "help":
            showHelp()
        case "list":
            listScripts()
        case "watch":
            if len(args) > 0 {
                watchScript(args[0])
            } else {
                fmt.Println(errorColor("请指定要监视的脚本"))
            }
        case "stop":
            if len(args) > 0 {
                stopProcess(args[0])
            } else {
                fmt.Println(errorColor("请指定要停止的进程"))
            }
        case "stopall":
            cleanupProcesses()
            fmt.Println(successColor("已停止所有进程"))
        default:
            if scriptCmd, ok := config.Scripts[command]; ok {
                runScript(command, args)
            } else {
                fmt.Printf("%s 未知命令: %s\n", errorColor("错误"), command)
            }
        }
    }
}

// 显示帮助
func showHelp() {
    fmt.Println(infoColor("\n可用命令:"))
    fmt.Println("  help    - 显示此帮助信息")
    fmt.Println("  list    - 列出所有可用脚本")
    fmt.Println("  exit    - 退出程序")
    fmt.Println("  watch   - 监视文件变化并执行脚本 (例如: watch start)")
    fmt.Println("  stop    - 停止指定名称的进程 (例如: stop start)")
    fmt.Println("  stopall - 停止所有运行的进程")
    fmt.Println("\n  或直接输入脚本名来运行该脚本 (例如: start)")
}

// 列出所有脚本
func listScripts() {
    fmt.Println(infoColor("\n可用脚本:"))
    for name, cmd := range config.Scripts {
        fmt.Printf("  %s - %s\n", name, cmd)
    }
}

// 运行脚本
func runScript(name string, args []string) {
    scriptCmd, ok := config.Scripts[name]
    if !ok {
        fmt.Printf("%s 未找到脚本: %s\n", errorColor("错误"), name)
        return
    }

    fmt.Printf("%s 执行: %s\n", infoColor("开始"), scriptCmd)

    // 添加脚本参数
    if len(args) > 0 {
        scriptCmd += " " + strings.Join(args, " ")
    }

    var cmd *exec.Cmd
    if runtime.GOOS == "windows" {
        cmd = exec.Command("cmd", "/C", scriptCmd)
    } else {
        cmd = exec.Command("sh", "-c", scriptCmd)
    }

    // 设置环境变量
    cmd.Env = os.Environ()
    for k, v := range config.Environment {
        cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
    }

    // 设置输出
    stdout, err := cmd.StdoutPipe()
    if err != nil {
        fmt.Printf("%s 无法获取标准输出: %v\n", errorColor("错误"), err)
        return
    }

    stderr, err := cmd.StderrPipe()
    if err != nil {
        fmt.Printf("%s 无法获取标准错误: %v\n", errorColor("错误"), err)
        return
    }

    // 启动命令
    if err := cmd.Start(); err != nil {
        fmt.Printf("%s 启动脚本失败: %v\n", errorColor("错误"), err)
        return
    }

    // 注册进程
    procMutex.Lock()
    processes[name] = cmd
    procMutex.Unlock()

    // 处理输出
    wg.Add(2)
    go printOutput(stdout, name, false)
    go printOutput(stderr, name, true)

    // 等待命令完成
    go func() {
        if err := cmd.Wait(); err != nil {
            fmt.Printf("%s 脚本 %s 执行失败: %v\n", errorColor("错误"), name, err)
        } else {
            fmt.Printf("%s 脚本 %s 执行完成\n", successColor("成功"), name)
        }

        // 移除进程
        procMutex.Lock()
        delete(processes, name)
        procMutex.Unlock()
    }()
}

// 监视文件变化并执行脚本
func watchScript(name string) {
    if _, ok := config.Scripts[name]; !ok {
        fmt.Printf("%s 未找到脚本: %s\n", errorColor("错误"), name)
        return
    }

    if len(config.WatchDirs) == 0 {
        fmt.Println(errorColor("未配置监视目录"))
        return
    }

    fmt.Printf("%s 开始监视文件变化...\n", infoColor("监视"))
    fmt.Printf("  目录: %s\n", strings.Join(config.WatchDirs, ", "))
    fmt.Printf("  扩展名: %s\n", strings.Join(config.WatchExts, ", "))

    // 初始化文件修改时间
    lastModTimes := make(map[string]time.Time)
    for _, dir := range config.WatchDirs {
        filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
            if err != nil {
                return nil
            }
            if !info.IsDir() {
                for _, ext := range config.WatchExts {
                    if strings.HasSuffix(path, "."+ext) {
                        lastModTimes[path] = info.ModTime()
                        break
                    }
                }
            }
            return nil
        })
    }

    // 开始监视
    go func() {
        for {
            time.Sleep(1 * time.Second)

            // 检查文件变化
            changed := false
            changedFiles := []string{}

            for _, dir := range config.WatchDirs {
                filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
                    if err != nil {
                        return nil
                    }
                    if !info.IsDir() {
                        for _, ext := range config.WatchExts {
                            if strings.HasSuffix(path, "."+ext) {
                                if t, ok := lastModTimes[path]; !ok || info.ModTime().After(t) {
                                    changedFiles = append(changedFiles, path)
                                    lastModTimes[path] = info.ModTime()
                                    changed = true
                                }
                                break
                            }
                        }
                    }
                    return nil
                })
            }

            if changed {
                fmt.Printf("\n%s 检测到文件变化: %s\n", infoColor("监视"), strings.Join(changedFiles, ", "))

                // 停止先前运行的进程
                stopProcess(name)

                // 等待一小段时间确保文件写入完成
                time.Sleep(300 * time.Millisecond)

                // 运行脚本
                runScript(name, []string{})
            }
        }
    }()
}

// 打印命令输出
func printOutput(pipe io.ReadCloser, prefix string, isError bool) {
    defer wg.Done()

    scanner := bufio.NewScanner(pipe)
    prefixColor := infoColor
    if isError {
        prefixColor = errorColor
    }

    for scanner.Scan() {
        line := scanner.Text()
        fmt.Printf("%s %s\n", prefixColor(prefix+":"), line)
    }
}

// 停止进程
func stopProcess(name string) {
    procMutex.Lock()
    defer procMutex.Unlock()

    cmd, ok := processes[name]
    if !ok {
        fmt.Printf("%s 没有正在运行的进程: %s\n", warnColor("警告"), name)
        return
    }

    if cmd.Process != nil {
        fmt.Printf("%s 停止进程: %s\n", infoColor("停止"), name)
        if err := cmd.Process.Kill(); err != nil {
            fmt.Printf("%s 无法停止进程 %s: %v\n", errorColor("错误"), name, err)
        } else {
            fmt.Printf("%s 进程已停止: %s\n", successColor("成功"), name)
        }
    }

    delete(processes, name)
}

// 清理所有进程
func cleanupProcesses() {
    procMutex.Lock()
    defer procMutex.Unlock()

    for name, cmd := range processes {
        if cmd.Process != nil {
            fmt.Printf("%s 停止进程: %s\n", infoColor("停止"), name)
            if err := cmd.Process.Kill(); err != nil {
                fmt.Printf("%s 无法停止进程 %s: %v\n", errorColor("错误"), name, err)
            }
        }
    }
    processes = make(map[string]*exec.Cmd)
}