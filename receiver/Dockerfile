FROM --platform=$BUILDPLATFORM brigadecore/go-tools:v0.8.0 as builder

ARG VERSION
ARG COMMIT
ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0

WORKDIR /src
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY receiver/ receiver/
COPY internal/ internal/

RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build \
  -o bin/receiver \
  -ldflags "-w -X github.com/brigadecore/brigade-foundations/version.version=$VERSION -X github.com/brigadecore/brigade-foundations/version.commit=$COMMIT" \
  ./receiver

FROM gcr.io/distroless/static:nonroot as final

COPY --from=builder /src/bin/ /brigade-github-gateway/bin/

ENTRYPOINT ["/brigade-github-gateway/bin/receiver"]
