FROM --platform=$BUILDPLATFORM golang:1.23 as builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /go/src/github.com/h3poteto/aws-global-accelerator-controller

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN set -ex && \
    TARGETOS="$TARGETOS" TARGETARCH="$TARGETARCH" \
    make build

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /go/src/github.com/h3poteto/aws-global-accelerator-controller/aws-global-accelerator-controller .
USER nonroot:nonroot

CMD ["/aws-global-accelerator-controller"]
