package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"

	"github.com/gin-gonic/gin"
)

// Node 代表一個 Chat Server 的狀態
type Node struct {
	ID     string `json:"server_id"`
	URL    string `json:"url"`
	Status string `json:"status"` // "alive", "dead", "frozen"
}

// 路由表結構（加鎖保護）
type RoutingTable struct {
	mu    sync.RWMutex
	Nodes map[string]*Node
}

var table = RoutingTable{
	Nodes: make(map[string]*Node),
}
var counter = 0

func main() {
	r := gin.Default()

	// 預設兩台測試機（先假裝它們活著）
	table.Nodes["server-1"] = &Node{ID: "server-1", URL: "http://localhost:9001", Status: "alive"}
	table.Nodes["server-2"] = &Node{ID: "server-2", URL: "http://localhost:9002", Status: "alive"}

	// ==========================================
	// 1. 真正的 Client 訊息轉發入口 (Reverse Proxy)
	// ==========================================
	r.POST("/send", func(c *gin.Context) {
		table.mu.RLock()
		var aliveNodes []*Node
		for _, node := range table.Nodes {
			if node.Status == "alive" {
				aliveNodes = append(aliveNodes, node)
			}
		}
		table.mu.RUnlock()

		if len(aliveNodes) == 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "所有 Chat Server 均無法連線"})
			return
		}

		// Round-Robin 選擇目標
		targetNode := aliveNodes[counter%len(aliveNodes)]
		counter++

		// 【核心升級】：解析目標 Chat Server 的 URL
		remote, err := url.Parse(targetNode.URL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "無效的後端 URL"})
			return
		}

		// 【核心升級】：建立反向代理，它會把整個 HTTP 請求原封不動轉發過去
		proxy := httputil.NewSingleHostReverseProxy(remote)
		
		// 執行轉發
		proxy.ServeHTTP(c.Writer, c.Request)
	})

	// ==========================================
	// 2. 為了架構完整，順便把歷史訊息查詢 /messages 也做轉發
	// ==========================================
	r.GET("/messages", func(c *gin.Context) {
		table.mu.RLock()
		var aliveNodes []*Node
		for _, node := range table.Nodes {
			if node.Status == "alive" {
				aliveNodes = append(aliveNodes, node)
			}
		}
		table.mu.RUnlock()

		if len(aliveNodes) == 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "所有 Chat Server 均無法連線"})
			return
		}

		targetNode := aliveNodes[counter%len(aliveNodes)]
		counter++

		remote, _ := url.Parse(targetNode.URL)
		proxy := httputil.NewSingleHostReverseProxy(remote)
		proxy.ServeHTTP(c.Writer, c.Request)
	})

	// ==========================================
	// 3. 作法 A：Heartbeat 狀態更新端點
	// ==========================================
	r.POST("/api/nodes/status", func(c *gin.Context) {
		var req Node
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "無效的格式"})
			return
		}

		table.mu.Lock()
		table.Nodes[req.ID] = &req
		table.mu.Unlock()

		c.JSON(http.StatusOK, gin.H{"message": "狀態更新成功", "current_table": table.Nodes})
	})

	r.Run(":8080")
}