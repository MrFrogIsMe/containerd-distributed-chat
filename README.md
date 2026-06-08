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
  │  限制容器實體記憶體 (64MB)、監聽 TaskExit event、協調 restart、凍結/恢復、通知 Gateway
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
| Failure Detection  | Gateway heartbeat registry| 停止傳送心跳 → 10s 後自動標 dead         |
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
安裝 Go：

```bash
sudo snap install go --classic
```

---

## 快速啟動

### 1. Build chat-server image

```bash
cd chat-server
docker build -t chat-server:latest .
docker save chat-server:latest | sudo ctr images import -
```

### 2. Run Gateway (視窗 1)

```bash
cd gateway
go run main.go
```

### 3. Run containerd Manager (視窗 2)

containerd Manager 會自動啟動三個 chat-server container，並套用 64MB 的記憶體限制：

```bash
cd containerd-manager
sudo go run main.go
```

### 4. 監控叢集狀態 (視窗 3)

專案提供了一套自動化監控與 Demo 腳本：

```bash
# 每 2 秒刷新一次 Gateway /status，用紅、黃、綠區分節點健康度
bash scripts/demo_status.sh
```

---

## Demo 情境與腳本

我們將所有測試流程寫成了自動化 Demo 腳本。在**視窗 4** 中，你可以依序執行以下腳本進行演示：

### Demo 1：正常聊天，round-robin 分流

```bash
bash scripts/demo_normal.sh
```
* **預期結果**：腳本會發送 9 條訊息，並顯示 server_id 輪流出現 `#1`、`#2`、`#3`，證明輪詢負載平衡完全正常。

### Demo 2：Freeze 節點，流量自動繞開

```bash
bash scripts/demo_freeze.sh
```
* **預期結果**：
  1. 暫停 `chat-server-2`，狀態在 `/status` 變為 `frozen`。
  2. 發送 6 條訊息，流量完全繞開，只由 `#1` 與 `#3` 回應。
  3. 恢復（Resume）`#2`，狀態回到 `alive`。
  4. 發送 6 條訊息，`#2` 自動重回輪替。

### Demo 3：Kill 節點，自動偵測並 restart (Auto-Recovery)

```bash
bash scripts/demo_kill.sh
```
* **預期結果**：直接向 containerd 發送 `SIGKILL` 幹掉 `chat-server-2` 容器。由於 containerd Manager 通過 `exitCh` 連接了 OCI event，Manager 會在 3 秒內清理並重建全新乾淨的容器，並自動重新註冊。

### Demo 4：OOM 觸發自動重啟 (OOM Auto-Recovery)

```bash
bash scripts/demo_oom.sh
```
* **預期結果**：
  1. `chat-server-2` 受限於 64MB 記憶體。
  2. 腳本在容器內注入約 150MB 的記憶體炸彈。
  3. Linux Kernel 的 OOM Killer 迅速終止容器（Exit code 137）。
  4. containerd Manager 偵測到死亡，於 3 秒內自動將其拉回。

---

## 手動操作與除錯指令

如果你不想使用自動化腳本，也可以手動執行以下核心指令進行 Demo：

```bash
# 查看當前 containerd 執行的 tasks
sudo ctr tasks list

# 手動凍結/解凍容器
curl -X POST "http://localhost:7000/freeze?id=chat-server-2"
curl -X POST "http://localhost:7000/resume?id=chat-server-2"

# 手動觸發 OOM-Kill 炸彈
sudo ctr tasks exec --exec-id manual-oom chat-server-2 python3 -c "x = 'A' * 150000000"
```

---

## 目錄結構

```
containerd-distributed-chat/
├── chat-server/          Python，聊天室服務（含 Dockerfile）
├── gateway/              Go，入口 + heartbeat registry
├── containerd-manager/   Go，容器生命週期管理與 event 訂閱
└── scripts/              Shell，自動化監控與一鍵 Demo 腳本
```
