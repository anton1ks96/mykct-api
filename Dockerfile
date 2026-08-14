ARG GO_VERSION=1.26.5
FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./

RUN go mod download

COPY . .

ENV CGO_ENABLED=0
ENV GOOS=linux

RUN go build -o mykct-api ./cmd/app

FROM scratch

WORKDIR /app

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /app/mykct-api ./mykct-api

EXPOSE 8080

ENTRYPOINT ["./mykct-api"]
