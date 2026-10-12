# Modified for this fork: current toolchain and cross-compilation for mixed fleets.
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine AS builder
RUN apk add -U --no-cache ca-certificates
RUN mkdir /runtime-tmp && chmod 1777 /runtime-tmp
WORKDIR /src

# Copy all source files
COPY . ./

ARG TARGETOS
ARG TARGETARCH
RUN go mod download
RUN CGO_ENABLED=0 go test ./pkg/...

# Build the adapter
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -mod=readonly -trimpath -tags=netgo -o /adapter ./cmd/adapter

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /adapter /adapter
COPY --from=builder --chmod=1777 /runtime-tmp/ /tmp/
COPY LICENSE NOTICE /licenses/
USER 1001:1001
ENTRYPOINT ["/adapter", "--logtostderr=true"]
