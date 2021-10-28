FROM --platform=$BUILDPLATFORM brigadecore/go-tools:v0.4.0 as builder

ARG VERSION
ARG COMMIT
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0

WORKDIR /src
COPY monitor/ monitor/
COPY internal/ internal/
COPY go.mod go.mod
COPY go.sum go.sum

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
  -o bin/monitor \
  -ldflags "-w -X github.com/brigadecore/brigade-foundations/version.version=$VERSION -X github.com/brigadecore/brigade-foundations/version.commit=$COMMIT" \
  ./monitor

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /src/bin/ /brigade-github-gateway/bin/
ENTRYPOINT ["/brigade-github-gateway/bin/monitor"]
