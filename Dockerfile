FROM golang:1.17.0 as builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -ldflags="-s -w" -o sample-controller

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/sample-controller /usr/local/bin
ENTRYPOINT ["sample-controller"]
