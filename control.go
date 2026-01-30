// control.go - VPS 控制端完整版（带华丽 GUI + 傻瓜化时间选择 + update_config 类型）
// 编译：go build -o control-server control.go
// 运行：./control-server -port=8080 -token=your-secret-token
// 访问：http://你的VPS:8080/

package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/gin-gonic/gin"
)

//go:embed static/*
var static embed.FS

var (
	tasks     []Task
	tasksMu   sync.RWMutex
	tasksFile = "tasks.json"
	token     string
)

// Task 与被控端一致，新增 Config 字段用于 update_config 类型
type Task struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Schedule string  `json:"schedule"` // "HH:MM" 格式，如 "09:30"
	Type     string  `json:"type"`
	Payload  string  `json:"payload,omitempty"`
	Config   *Config `json:"config,omitempty"` // 只在 update_config 时使用
}

type Config struct {
	RemoteURL string `json:"remote_url"`
	AuthToken string `json:"auth_token"`
}

func main() {
	port := flag.String("port", "8080", "监听端口")
	tok := flag.String("token", "secret123", "认证 Token")
	flag.Parse()
	token = *tok

	loadTasks()

	r := gin.Default()

	// 认证中间件
	auth := func(c *gin.Context) {
		if c.GetHeader("Authorization") != "Bearer "+token {
			c.JSON(401, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}

	api := r.Group("/api")
	api.Use(auth)
	{
		api.GET("/tasks", getTasks)
		api.POST("/tasks", overwriteTasks)
		api.POST("/tasks/add", addTask)
		api.PUT("/tasks/:id", updateTask)
		api.DELETE("/tasks/:id", deleteTask)
	}

	// 静态文件（嵌入的 HTML/JS）
	staticFS, _ := fs.Sub(static, "static")
	r.StaticFS("/static", http.FS(staticFS))
	r.GET("/", func(c *gin.Context) {
		c.Redirect(302, "/static/index.html")
	})

	log.Printf("控制端启动于 :%s | Token: %s", *port, token)
	r.Run(":" + *port)
}

func loadTasks() {
	data, err := os.ReadFile(tasksFile)
	if err == nil {
		json.Unmarshal(data, &tasks)
	}
}

func saveTasks() {
	data, _ := json.MarshalIndent(tasks, "", "  ")
	os.WriteFile(tasksFile, data, 0644)
}

func getTasks(c *gin.Context) {
	tasksMu.RLock()
	c.JSON(200, tasks)
	tasksMu.RUnlock()
}

func overwriteTasks(c *gin.Context) {
	var newTasks []Task
	if err := c.BindJSON(&newTasks); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	tasksMu.Lock()
	tasks = newTasks
	tasksMu.Unlock()
	saveTasks()
	c.JSON(200, gin.H{"status": "updated"})
}

func addTask(c *gin.Context) {
	var t Task
	if err := c.BindJSON(&t); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	tasksMu.Lock()
	tasks = append(tasks, t)
	tasksMu.Unlock()
	saveTasks()
	c.JSON(200, gin.H{"status": "added"})
}

func updateTask(c *gin.Context) {
	id := c.Param("id")
	var updated Task
	if err := c.BindJSON(&updated); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	tasksMu.Lock()
	for i := range tasks {
		if tasks[i].ID == id {
			tasks[i] = updated
			break
		}
	}
	tasksMu.Unlock()
	saveTasks()
	c.JSON(200, gin.H{"status": "updated"})
}

func deleteTask(c *gin.Context) {
	id := c.Param("id")
	tasksMu.Lock()
	newTasks := []Task{}
	for _, t := range tasks {
		if t.ID != id {
			newTasks = append(newTasks, t)
		}
	}
	tasks = newTasks
	tasksMu.Unlock()
	saveTasks()
	c.JSON(200, gin.H{"status": "deleted"})
}
