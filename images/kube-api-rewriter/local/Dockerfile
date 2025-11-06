# Local development image for kube-api-rewriter.
# Not used by the production build.

FROM golang:1.24.3-alpine3.20 AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -o /tmp/kube-api-rewriter ./cmd/kube-api-rewriter

FROM alpine:3.20
RUN apk add --no-cache ca-certificates bash tini

COPY --from=builder /tmp/kube-api-rewriter /usr/local/bin/kube-api-rewriter
COPY local/kube-api-rewriter.kubeconfig /etc/kube-api-rewriter/kubeconfig

ENV PROXY_LISTEN_ADDR=0.0.0.0:23915 \
    METRICS_LISTEN_ADDR=0.0.0.0:9090

ENTRYPOINT ["/sbin/tini","--"]
CMD ["/usr/local/bin/kube-api-rewriter"]
