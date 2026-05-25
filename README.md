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
- Python 3.11+（Chat Server）

安裝 containerd（Ubuntu/Debian）：

```bash
apt install containerd
systemctl enable --now containerd
```

---

## 快速啟動

```bash
# 1. 啟動所有元件
./scripts/start_all.sh

# 2. 查節點狀態
curl http://localhost:8080/status

# 3. 送訊息（會 round-robin 分流）
curl -X POST http://localhost:8080/send \
  -H 'Content-Type: application/json' \
  -d '{"user":"alice","message":"hello"}'
```

---

## Demo 情境

```bash
# Demo 1：正常運作，訊息輪流由 #1/#2/#3 回應
./scripts/demo_normal.sh

# Demo 2：freeze #1，流量自動繞開，resume 後恢復
./scripts/demo_freeze.sh

# Demo 3：kill #2，Gateway 偵測 dead，containerd 自動 restart
./scripts/demo_kill.sh
```

---

## 文件索引

| 檔案 | 說明 |
|------|------|
| [docs/PLAN.md](docs/PLAN.md) | 系統架構書：問題動機、元件職責、API 規格、分工時程 |
| [docs/final_demo_requirements.md](docs/final_demo_requirements.md) | 期末 Demo 規定 |

---

## 目錄結構

```
final/
├── chat-server/          Python，聊天室服務（含 Dockerfile）
├── gateway/              Go，入口 + heartbeat registry
├── containerd-manager/   Go，容器生命週期管理與 event 訂閱
└── scripts/              Demo 與測試腳本
```
