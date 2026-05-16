# syntax=docker/dockerfile:1.7

# ----------------------------------------------------------------------
# Stage 1: deps
# go.mod / go.sum だけをコピーして依存を先に解決する
# ----------------------------------------------------------------------
ARG GO_VERSION=1.25
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS deps
WORKDIR /src

RUN apk add --no-cache git ca-certificates tzdata
COPY go.mod go.sum* ./
RUN go mod download

# ----------------------------------------------------------------------
# Stage 2: builder
# ソースを取り込み server と worker の2つの static バイナリを作る
# ----------------------------------------------------------------------
FROM deps AS builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY . .

ENV CGO_ENABLED=0
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

# ----------------------------------------------------------------------
# Stage 3: runtime
# distroless static で nonroot で動かす
# テンプレートと静的アセットは embed.FS でバイナリに同梱されている
# migrations は /app/migrations へコピーしてアプリ起動時に参照する
# ----------------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app

COPY --from=builder /out/server /app/server
COPY --from=builder /out/worker /app/worker
COPY --from=builder /src/migrations /app/migrations

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app/server"]
