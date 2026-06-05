package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type Node struct {
	ID            string    `json:"server_id"`
	URL           string    `json:"url"`
	Status        string    `json:"status"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

type RegisterReq struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
}

type RoutingTable struct {
	mu    sync.RWMutex
	Nodes map[string]*Node
}

var table = RoutingTable{Nodes: make(map[string]*Node)}
var counter atomic.Int64

const heartbeatTimeout = 10 * time.Second
const checkInterval = 3 * time.Second

func startWatchdog() {
	ticker := time.NewTicker(checkInterval)
	go func() {
		for range ticker.C {
			now := time.Now()
			table.mu.Lock()
			for id, node := range table.Nodes {
				if node.Status == "frozen" || node.Status == "dead" {
					continue
				}
				if now.Sub(node.LastHeartbeat) > heartbeatTimeout {
					log.Printf("[Watchdog] 節點 %s 超過 %.0fs 沒有 heartbeat，標記為 dead", id, heartbeatTimeout.Seconds())
					node.Status = "dead"
				}
			}
			table.mu.Unlock()
		}
	}()
	log.Printf("[Watchdog] 啟動，每 %.0fs 掃一次，超過 %.0fs 沒心跳標 dead", checkInterval.Seconds(), heartbeatTimeout.Seconds())
}

func main() {
	startWatchdog()
	r := gin.Default()

	r.POST("/send", func(c *gin.Context) {
		node := pickAliveNode()
		if node == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "所有 Chat Server 均無法連線"})
			return
		}
		proxyTo(node.URL, c)
	})

	r.GET("/messages", func(c *gin.Context) {
		node := pickAliveNode()
		if node == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"ok": false, "error": "所有 Chat Server 均無法連線"})
			return
		}
		proxyTo(node.URL, c)
	})

	r.POST("/register", func(c *gin.Context) {
		var req RegisterReq
		if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" || req.Addr == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "需要 id 和 addr"})
			return
		}
		table.mu.Lock()
		table.Nodes[req.ID] = &Node{
			ID:            req.ID,
			URL:           "http://" + req.Addr,
			Status:        "alive",
			LastHeartbeat: time.Now(),
		}
		table.mu.Unlock()
		log.Printf("[Gateway] 節點 %s 已註冊 (%s)", req.ID, req.Addr)
		c.JSON(http.StatusOK, gin.H{"ok": true, "message": "註冊成功"})
	})

	r.POST("/heartbeat", func(c *gin.Context) {
		var req struct {
			ID string `json:"id"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"ok": false, "error": "需要 id"})
			return
		}
		table.mu.Lock()
		node, exists := table.Nodes[req.ID]
		if !exists {
			table.mu.Unlock()
			c.JSON(http.StatusNotFound, gin.H{"ok": false, "error": "節點未註冊"})
			return
		}
		node.LastHeartbeat = time.Now()
		if node.Status == "dead" {
			node.Status = "alive"
			log.Printf("[Gateway] 節點 %s 心跳恢復，標記為 alive", req.ID)
		}
		table.mu.Unlock()
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.POST("/api/nodes/status", func(c *gin.Context) {
		var req Node
		if err := c.ShouldBindJSON(&req); err != nil || req.ID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "無效的格式"})
			return
		}
		table.mu.Lock()
		if existing, ok := table.Nodes[req.ID]; ok {
			existing.Status = req.Status
			if req.URL != "" {
				existing.URL = req.URL
			}
		} else {
			req.LastHeartbeat = time.Now()
			table.Nodes[req.ID] = &req
		}
		table.mu.Unlock()
		log.Printf("[Gateway] containerd Manager 更新：%s → %s", req.ID, req.Status)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r.GET("/status", func(c *gin.Context) {
		table.mu.RLock()
		defer table.mu.RUnlock()
		result := make(map[string]gin.H, len(table.Nodes))
		for id, node := range table.Nodes {
			result[id] = gin.H{
				"status":         node.Status,
				"url":            node.URL,
				"last_heartbeat": node.LastHeartbeat.Format(time.RFC3339),
				"seconds_ago":    int(time.Since(node.LastHeartbeat).Seconds()),
			}
		}
		c.JSON(http.StatusOK, result)
	})

	r.Run(":8080")
}

func pickAliveNode() *Node {
	table.mu.RLock()
	defer table.mu.RUnlock()
	var alive []*Node
	for _, node := range table.Nodes {
		if node.Status == "alive" {
			alive = append(alive, node)
		}
	}
	if len(alive) == 0 {
		return nil
	}
	n := counter.Add(1) - 1
	return alive[n%int64(len(alive))]
}

func proxyTo(targetURL string, c *gin.Context) {
	remote, _ := url.Parse(targetURL)
	proxy := httputil.NewSingleHostReverseProxy(remote)
	proxy.ServeHTTP(c.Writer, c.Request)
}
