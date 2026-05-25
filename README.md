# 分散式聊天室 — containerd 容器協調示範

以 containerd 為底層 runtime，自行實作容器生命週期管理、heartbeat 偵測、自動重啟的分散式聊天室系統。

---

## 系統架構

```
Client
  │
  ▼
Gateway :8080        ← 入口、round-robin 分流、dead/frozen 判斷
  │
  ├── Chat Server #1 :9001
  ├── Chat Server #2 :9002
  └── Chat Server #3 :9003
        │
        │ heartbeat / register
        ▼
  Gateway ServerRegistry

containerd Manager :7000   ← 操控 containerd daemon，管容器生命週期
  │
  ▼
containerd daemon (/run/containerd/containerd.sock)
```

---

## 分散式模式

| 模式               | 實作位置          | Demo 方式                          |
|--------------------|-------------------|------------------------------------|
| Failure Detection  | Gateway heartbeat | kill server → 10s 後標 dead        |
| Auto Recovery      | containerd Manager| container exit → 自動 restart      |
| Load Balancing     | Gateway round-robin| 訊息輪流由不同 server 回應         |
| Freeze / Resume    | containerd Manager| task.Pause() / Resume()            |

---

## 環境需求

- containerd daemon（需預先安裝，`apt install containerd`）
- Go 1.21+（Gateway、containerd Manager）
- Python 3.11+（Chat Server）

---

## 快速啟動

```bash
# 1. 啟動所有元件
./scripts/start_all.sh

# 2. 查節點狀態
curl http://localhost:8080/status

# 3. 送訊息
curl -X POST http://localhost:8080/send \
  -H 'Content-Type: application/json' \
  -d '{"user":"alice","message":"hello"}'

# 4. Demo：freeze 一個節點
curl -X POST http://localhost:7000/container/freeze -d '{"id":"#1"}'

# 5. Demo：kill 一個節點，觀察自動重啟
curl -X POST http://localhost:7000/container/stop -d '{"id":"#2"}'
```

---

## 文件

| 檔案 | 說明 |
|------|------|
| [docs/PLAN.md](docs/PLAN.md) | 系統架構書：元件職責、API 規格、交付物、分工時程 |
| [docs/final_demo_requirements.md](docs/final_demo_requirements.md) | 期末 Demo 規定 |

---

## 目錄結構

```
final/
├── chat-server/          Python，聊天室服務
├── gateway/              Go，入口 + heartbeat registry
├── containerd-manager/   Go，容器生命週期管理
└── scripts/              Demo 與測試腳本
```
