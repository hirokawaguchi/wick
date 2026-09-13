FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /wick ./cmd/wick
RUN CGO_ENABLED=0 go build -o /websearch-mcp ./cmd/websearch-mcp

# --- 本体（BBS 局） ---
FROM alpine:3.20 AS wick
RUN apk add --no-cache tzdata \
	&& adduser -D -H -u 1000 wick
WORKDIR /app
COPY --from=build /wick /usr/local/bin/wick
COPY data /app/asset
RUN mkdir -p /data && chown wick:wick /data
USER wick
ENV TZ=Asia/Tokyo
ENV WICK_TZ=Asia/Tokyo
ENV WICK_DATA=/data
ENV WICK_ASSET=/app/asset
ENV WICK_LISTEN=:2222
EXPOSE 2222
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/wick"]

# --- web 検索 MCP サーバ（エージェント専用・別サービス） ---
FROM alpine:3.20 AS websearch
RUN apk add --no-cache ca-certificates tzdata \
	&& adduser -D -H -u 1001 websearch
COPY --from=build /websearch-mcp /usr/local/bin/websearch-mcp
USER websearch
ENV TZ=Asia/Tokyo
ENV WEBSEARCH_LISTEN=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/websearch-mcp"]
