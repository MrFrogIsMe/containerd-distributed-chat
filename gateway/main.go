package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"

	"github.com/gin-gonic/gin"
)

type Node struct {
	ID     string `json:"server_id"`
	URL    string `json:"url"`
	Status string `json:"status"` // "alive", "dead", "frozen"
}

// 接收隊友註冊時的 JSON 格式
type RegisterReq struct {
	ID   string `json:"id"`
	Addr string `json:"addr"` // 例如 "localhost:9001"
}

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

	// ==========================================
	// 1. Client 訊息轉發與歷史訊息入口
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
			c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "所有 Chat Server 均無法連線"})
			return
		}

		targetNode := aliveNodes[counter%len(aliveNodes)]
		counter++

		remote, _ := url.Parse(targetNode.URL)
		proxy := httputil.NewSingleHostReverseProxy(remote)
		proxy.ServeHTTP(c.Writer, c.Request)
	})

	r.GET("/messages", func(c *gin.Context) {
		// 邏輯同上，複製過來即可
		table.mu.RLock()
		var aliveNodes []*Node
		for _, node := range table.Nodes {
			if node.Status == "alive" {
				aliveNodes = append(aliveNodes, node)
			}
		}
		table.mu.RUnlock()

		if len(aliveNodes) == 0 {
			c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "所有 Chat Server 均無法連線"})
			return
		}

		targetNode := aliveNodes[counter%len(aliveNodes)]
		counter++

		remote, _ := url.Parse(targetNode.URL)
		proxy := httputil.NewSingleHostReverseProxy(remote)
		proxy.ServeHTTP(c.Writer, c.Request)
	})

	// ==========================================
	// 2. 【新增】配合 Chat Server 負責人的主動註冊端點
	// ==========================================
	r.POST("/register", func(c *gin.Context) {
		var req RegisterReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "無效的註冊格式"})
			return
		}

		table.mu.Lock()
		// 把隊友傳來的 "localhost:9001" 轉成 "http://localhost:9001"
		table.Nodes[req.ID] = &Node{
			ID:     req.ID,
			URL:    "http://" + req.Addr,
			Status: "alive", // 註冊進來預設是活著
		}
		table.mu.Unlock()

		c.JSON(http.StatusOK, gin.H{"ok": true, "message": "Chat Server 註冊成功"})
	})

	// ==========================================
	// 3. 【新增】配合 Chat Server 負責人的 Heartbeat 端點
	// ==========================================
	r.POST("/heartbeat", func(c *gin.Context) {
		var req map[string]string // 隊友只傳 {"id": "#1"}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false})
			return
		}

		serverID := req["id"]
		println(serverID)
		table.mu.Lock()
		if node, exists := table.Nodes[serverID]; exists {
			node.Status = "alive" // 收到心跳，確保他是 alive
		}
		table.mu.Unlock()

		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// ==========================================
	// 4. 原本留給 containerd 密你的端點（作法 A 依然保留！）
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

		c.JSON(http.StatusOK, gin.H{"message": "containerd 狀態更新成功"})
	})

	r.Run(":8080")
}