# 安定版のGo 1.26 を使用
FROM golang:1.26.2

# SCTP通信に必要なOSパッケージ
RUN apt-get update && apt-get install -y libsctp-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# 依存モジュールのキャッシュを効かせるため、先にgo.mod等をコピー
COPY go.mod go.sum ./
RUN go mod download

# ソースコード全体をコピー
COPY . .

# C-Plane Gatewayをビルド
RUN go build -o cgw ./cmd/cgw

# 実行
CMD ["./cgw"]
