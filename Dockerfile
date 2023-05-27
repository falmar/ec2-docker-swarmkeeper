FROM golang:1.19-alpine AS worker-builder
WORKDIR /app
COPY go.mod go.sum /app/
RUN go mod download
COPY cmd /app/cmd
COPY internal /app/internal
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o main ./cmd

FROM alpine:3.18 as worker
RUN apk add --no-cache tini ca-certificates
WORKDIR /app
COPY --from=builder /app/main /app/main
ENTRYPOINT ["./main", "worker"]

FROM alpine:3.18 as manager
RUN apk add --no-cache tini ca-certificates
WORKDIR /app
COPY --from=builder /app/main /app/main
ENTRYPOINT ["./main", "manager"]

FROM alpine:3.18 as metadata
RUN apk add --no-cache tini ca-certificates
WORKDIR /app
COPY --from=builder /app/main /app/main
ENTRYPOINT ["./main", "metadata"]
