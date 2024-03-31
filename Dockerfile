FROM golang:1.22 as builder

ENV CGO_ENABLED="0" \
    GOOS="linux" \
    GOARCH="amd64"

WORKDIR /go/src/github.com/h3poteto/aws-global-accelerator-controller

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN set -ex && \
    make build

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /go/src/github.com/h3poteto/aws-global-accelerator-controller/aws-global-accelerator-controller .
USER nonroot:nonroot

CMD ["/aws-global-accelerator-controller"]
