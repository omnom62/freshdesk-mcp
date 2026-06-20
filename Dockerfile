# Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o freshdesk-mcp .

# Runtime stage
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/freshdesk-mcp .
ENV MCP_TRANSPORT=http
EXPOSE 8080
CMD ["./freshdesk-mcp"]
