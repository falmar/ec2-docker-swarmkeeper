FROM golang:1.19-alpine AS builder
WORKDIR /app
COPY go.mod go.sum main.go /app/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o main .

FROM alpine:3.18
RUN apk add --no-cache tini ca-certificates
WORKDIR /app
COPY --from=builder /app/main /app/main
ENTRYPOINT ["./main"]
