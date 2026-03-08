# 系統設計作業題：社群短網址服務

題目：設計一個高可用的短網址服務（Short URL Service），應用於社群分享場景
請設計一個短網址系統，當使用者在社群平台（如 Facebook, Thread, A-pen）分享內容時，能夠產生短網址並提供豐富的分享體驗。系統需滿足以下核心需求：

## 功能需求（Functional Requirements）

### 1. 短網址生成與導向：將長網址轉換為短網址，並能正確重新導向至原始頁面。

- 短網址 id: Base58
- POC 先使用 snowflakeID, 長度 10 碼
- 後續優化成分配區段

### 2. 連結預覽（Link Preview）：短網址在社群平台貼上時，需能正確呈現預覽資訊（標題、摘要、縮圖等），提供良好的分享體驗。

- 建立短網址時需要去爬, 建立 og 資訊
- api service 建立短網址後送 msg 出去, 由 worker 非同步建立

### 3. 推薦碼機制（Referral Tracking）：使用者分享短網址時，可攜帶個人推薦碼，系統需能識別並記錄推薦來源。

- url 綁定一個 userID
- 可以埋 pixel

### 4. 成效追蹤與歸因分析（Analytics & Attribution）：記錄每個短網址的點擊數據（點擊次數、來源平台、地區、裝置等），並能將轉換成效歸因至特定推薦者或行銷活動。

- 點擊短網址時要非同步送 user metadata 到 BQ

## 非功能需求（Non-Functional Requirements）

5. 高可用性（High Availability）：服務需具備高可用性，短網址的導向不可因單點故障而中斷。
6. 低延遲（Low Latency）：短網址的重新導向以及產生應在極短時間內完成。
7. 可擴展性（Scalability）：需能承受社群分享帶來的突發流量高峰。
