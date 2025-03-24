package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings" // 新增导入
	"sync"
	"time"
)

// 用户数据模型
type User struct {
    ID        int       `json:"id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    CreatedAt time.Time `json:"created_at"`
}

// 响应包装器
type ApiResponse struct {
    Success bool        `json:"success"`
    Data    interface{} `json:"data,omitempty"`
    Error   string      `json:"error,omitempty"`
}

// 简单的内存数据库
type UserStore struct {
    sync.RWMutex
    users  map[int]User
    nextID int
}

// 新建用户存储
func NewUserStore() *UserStore {
    return &UserStore{
        users:  make(map[int]User),
        nextID: 1,
    }
}

// 创建用户
func (s *UserStore) Create(user User) User {
    s.Lock()
    defer s.Unlock()

    user.ID = s.nextID
    user.CreatedAt = time.Now()
    s.users[user.ID] = user
    s.nextID++

    return user
}

// 获取所有用户
func (s *UserStore) GetAll() []User {
    s.RLock()
    defer s.RUnlock()

    users := make([]User, 0, len(s.users))
    for _, user := range s.users {
        users = append(users, user)
    }
    return users
}

// 根据ID获取用户
func (s *UserStore) GetByID(id int) (User, bool) {
    s.RLock()
    defer s.RUnlock()

    user, exists := s.users[id]
    return user, exists
}

// 更新用户
func (s *UserStore) Update(id int, user User) (User, bool) {
    s.Lock()
    defer s.Unlock()

    existing, exists := s.users[id]
    if !exists {
        return User{}, false
    }

    // 保持ID和创建时间不变
    user.ID = existing.ID
    user.CreatedAt = existing.CreatedAt
    s.users[id] = user

    return user, true
}

// 删除用户
func (s *UserStore) Delete(id int) bool {
    s.Lock()
    defer s.Unlock()

    _, exists := s.users[id]
    if exists {
        delete(s.users, id)
    }
    return exists
}

// 日志中间件
func loggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next.ServeHTTP(w, r)
        log.Printf("%s %s %s", r.Method, r.RequestURI, time.Since(start))
    })
}

func main() {
    // 命令行参数
    port := flag.Int("port", 8080, "API服务器端口")
    flag.Parse()

    // 初始化数据存储
    store := NewUserStore()

    // 添加一些示例数据
    store.Create(User{Name: "张三", Email: "zhang@example.com"})
    store.Create(User{Name: "李四", Email: "li@example.com"})
    store.Create(User{Name: "王五", Email: "wang@example.com"})

    // 创建路由
    mux := http.NewServeMux()

    // 处理 /users 路由（获取所有用户和创建用户）
    mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
        // 确保是根路径 /users 而不是 /users/anything
        if r.URL.Path != "/users" {
            http.NotFound(w, r)
            return
        }
        
        switch r.Method {
        case http.MethodGet:
            // 获取所有用户
            users := store.GetAll()
            sendJSON(w, ApiResponse{Success: true, Data: users})
            
        case http.MethodPost:
            // 创建用户
            var user User
            if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
                sendError(w, "无效的请求数据", http.StatusBadRequest)
                return
            }
            
            createdUser := store.Create(user)
            sendJSON(w, ApiResponse{Success: true, Data: createdUser})
            
        default:
            http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
        }
    })

    // 处理 /users/{id} 路由（获取、更新、删除单个用户）
    mux.HandleFunc("/users/", func(w http.ResponseWriter, r *http.Request) {
        // 从路径中提取ID
        pathParts := strings.Split(r.URL.Path, "/")
        if len(pathParts) != 3 {
            sendError(w, "无效的路径", http.StatusBadRequest)
            return
        }
        
        idStr := pathParts[2]
        id, err := strconv.Atoi(idStr)
        if err != nil {
            sendError(w, "无效的用户ID", http.StatusBadRequest)
            return
        }
        
        switch r.Method {
        case http.MethodGet:
            // 获取单个用户
            user, exists := store.GetByID(id)
            if !exists {
                sendError(w, "用户不存在", http.StatusNotFound)
                return
            }
            
            sendJSON(w, ApiResponse{Success: true, Data: user})
            
        case http.MethodPut:
            // 更新用户
            var user User
            if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
                sendError(w, "无效的请求数据", http.StatusBadRequest)
                return
            }
            
            updatedUser, exists := store.Update(id, user)
            if !exists {
                sendError(w, "用户不存在", http.StatusNotFound)
                return
            }
            
            sendJSON(w, ApiResponse{Success: true, Data: updatedUser})
            
        case http.MethodDelete:
            // 删除用户
            success := store.Delete(id)
            if !success {
                sendError(w, "用户不存在", http.StatusNotFound)
                return
            }
            
            sendJSON(w, ApiResponse{Success: true})
            
        default:
            http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
        }
    })

    // 应用中间件
    handler := loggingMiddleware(mux)

    // 启动服务器
    addr := fmt.Sprintf(":%d", *port)
    fmt.Printf("API 服务器启动在 http://localhost%s\n", addr)
    log.Fatal(http.ListenAndServe(addr, handler))
}

// 辅助函数：发送JSON响应
func sendJSON(w http.ResponseWriter, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(data)
}

// 辅助函数：发送错误响应
func sendError(w http.ResponseWriter, message string, statusCode int) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(ApiResponse{
        Success: false,
        Error:   message,
    })
}