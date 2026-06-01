# 分散式聊天室 — containerd 容器協調示範

## 問題背景

假設你維運一個即時聊天服務，部署在一台機器上用三個 Chat Server 分流負載。
現實中會遇到三個問題：

```
問題一：Chat Server 因 bug 或 OOM 崩掉
        → 那個 port 的流量永遠進不來，要人工重啟

問題二：需要滾動更新其中一個 server
        → 直接停掉會讓進行中的請求失敗

問題三：凌晨沒人值班，server 掛掉沒人知道
        → 需要自動偵測 + 自動恢復，不依賴人工介入
```

containerd 可以管理 container 的生命週期（start / stop / pause / resume），
但它本身**不偵測服務健康、不跟 Gateway 聯動、不會自動協調恢復**。

我們在 containerd 之上實作一層協調系統，讓服務在 container 故障時
對 client 透明地自癒——這就是這個專題想解決的問題。

---

## 系統架構

```
Client (curl / browser)
  │ HTTP
  ▼
Gateway :8080          ← 入口、round-robin 分流
  │                       偵測 dead/frozen，自動繞開故障節點
  ├── Chat Server #1 :9001  ┐
  ├── Chat Server #2 :9002  ├── 各自向 Gateway 送 heartbeat（每 3 秒）
  └── Chat Server #3 :9003  ┘

containerd Manager :7000
  │  訂閱 TaskExit event、協調 restart、凍結/恢復、通知 Gateway
  ▼
containerd daemon (/run/containerd/containerd.sock)
  │
  └── 實際跑三個 Chat Server container
```

節點清單：

| 元件               | Port | 語言   | 說明                            |
|--------------------|------|--------|---------------------------------|
| Gateway            | 8080 | Go     | 入口、分流、heartbeat registry  |
| Chat Server #1-#3  | 9001-9003 | Python | 聊天室服務實體             |
| containerd Manager | 7000 | Go     | 容器生命週期管理與協調          |

> 三個元件跑在同一台機器，透過 localhost:port 網路溝通，
> 符合「至少 3 個節點，只透過網路溝通」的規定。

---

## 分散式系統模式

| 模式               | 實作位置                  | Demo 方式                               |
|--------------------|---------------------------|-----------------------------------------|
| Failure Detection  | Gateway heartbeat registry| kill server → 10s 後自動標 dead         |
| Auto Recovery      | containerd Manager        | container exit event → 自動 restart     |
| Load Balancing     | Gateway round-robin       | 訊息輪流由不同 server 回應              |
| Freeze / Resume    | containerd Manager        | task.Pause() / Resume() + Gateway 聯動  |

**為什麼用 container 而不是直接跑 process？**

- 生命週期有標準介面：start / stop / pause / resume 全部走 containerd API
- Freeze 有系統原語：task.Pause() 底層是 cgroup freezer，比 SIGSTOP 更乾淨
- Restart 後是乾淨的環境：image 保證每次啟動狀態一致
- 統一的 event 系統：訂閱 TaskExit 就能感知所有 container 的死亡

---

## 環境需求

- Linux（containerd 不支援 macOS 原生執行）
- containerd v2.x（需 root 或 containerd socket 存取權限）
- runc（containerd 的預設 OCI runtime）
- Go 1.21+（Gateway、containerd Manager）
- Python 3.11+（Chat Server，本機開發用）
- Docker（用來 build chat-server image，再匯入 containerd）

安裝 containerd（Ubuntu/Debian）：

```bash
sudo apt install containerd
sudo systemctl enable --now containerd
```

---

## 快速啟動

### 1. Build chat-server image

```bash
cd chat-server
docker build -t chat-server:latest .
docker save chat-server:latest | sudo ctr images import -
```

### 2. Run Gateway

```bash
cd gateway
go run main.go
```

### 3. Run containerd Manager

containerd Manager 會自動啟動三個 chat-server container：

```bash
cd containerd-manager
sudo go run main.go
```

成功後會看到：

```
started chat-server-1 (SERVER_ID=#1, port=9001, pid=...)
started chat-server-2 (SERVER_ID=#2, port=9002, pid=...)
started chat-server-3 (SERVER_ID=#3, port=9003, pid=...)
containerd manager listening on :7000
```

### 測試指令

```bash
# 送訊息（會 round-robin 分流）
curl -X POST http://localhost:8080/send \
  -H 'Content-Type: application/json' \
  -d '{"user":"alice","message":"hello"}'

# 查訊息
curl http://localhost:8080/messages

# 直接檢查某一個 Chat Server
curl http://localhost:9001/health
```

---

## Demo 情境

### Demo 1：正常聊天，round-robin 分流

```bash
for i in $(seq 1 6); do
  curl -s -X POST http://localhost:8080/send \
    -H 'Content-Type: application/json' \
    -d "{\"user\":\"alice\",\"message\":\"msg $i\"}"
  echo
done
```

訊息會輪流由 #1 / #2 / #3 回應。

### Demo 2：Freeze 節點，流量自動繞開

```bash
# freeze #2
curl -X POST "http://localhost:7000/freeze?id=chat-server-2"

# 送訊息，只會由 #1 和 #3 回應
curl -X POST http://localhost:8080/send \
  -H 'Content-Type: application/json' \
  -d '{"user":"alice","message":"frozen test"}'

# resume #2
curl -X POST "http://localhost:7000/resume?id=chat-server-2"
```

### Demo 3：Kill 節點，自動偵測並 restart

```bash
# kill chat-server-2 的 container
sudo ctr tasks kill -s SIGKILL chat-server-2

# 觀察 containerd Manager 的 terminal，3 秒後自動 restart
# [EXIT] chat-server-2 exited with code 137 — restarting in 3s
# started chat-server-2 (SERVER_ID=#2, port=9002, pid=...)
```

---

## 目錄結構

```
containerd-distributed-chat/
├── chat-server/          Python，聊天室服務（含 Dockerfile）
├── gateway/              Go，入口 + heartbeat registry
└── containerd-manager/   Go，容器生命週期管理與 event 訂閱
```
