# Build the manager binary
FROM golang:1.19.4-alpine3.16 as builder
ARG LDFLAGS
ARG PKGNAME
ARG BUILD
ENV GO111MODULE=on \
    CGO_ENABLED=0
WORKDIR /app

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN if [[ "${BUILD}" != "CI" ]]; then go env -w GOPROXY=https://goproxy.io,direct; fi
RUN go env
RUN go mod download

# Copy the go source
COPY api api/
COPY pkg pkg/
COPY cmd cmd/

# Build
RUN env
RUN go build -ldflags="${LDFLAGS}" -a -o httpbin cmd/main.go

FROM alpine:3.15.3
WORKDIR /app
ARG PKGNAME
EXPOSE 80
COPY --from=builder /app/httpbin .
CMD ["./httpbin"]
