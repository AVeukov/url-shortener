FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/url-shortener ./cmd/api

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/url-shortener /usr/local/bin/url-shortener
USER nobody
EXPOSE 8080
ENTRYPOINT ["url-shortener"]
