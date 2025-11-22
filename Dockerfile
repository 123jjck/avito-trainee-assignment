FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o pr-service ./cmd/server

FROM alpine:3.22

RUN adduser -D app
USER app

WORKDIR /home/app
COPY --from=builder /app/pr-service /usr/local/bin/pr-service

ENV PORT=8080
CMD ["/usr/local/bin/pr-service"]
