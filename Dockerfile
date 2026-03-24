# 第一階段：編譯
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# 複製依賴檔案
COPY go.mod go.sum ./
RUN go mod download

# 複製原始碼
COPY . .

# 編譯執行檔
RUN go build -o paylock ./cmd/paylock

# 第二階段：執行環境
FROM debian:bookworm-slim

# 安裝 ffmpeg 與必要套件
RUN apt-get update && apt-get install -y \
    ffmpeg \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 從編譯階段複製執行檔
COPY --from=builder /app/paylock .

# 設定環境變數預設值
ENV PAYLOCK_PORT=8080
ENV PAYLOCK_DATA_DIR=/data

# 建立資料夾
RUN mkdir -p /data

# 暴露埠號
EXPOSE 8080

# 啟動指令
CMD ["./paylock"]
