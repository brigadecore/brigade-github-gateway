FROM brigadecore/go-tools:v0.1.0
ARG VERSION
ARG COMMIT
ENV CGO_ENABLED=0
WORKDIR /src
COPY receiver/ receiver/
COPY internal/ internal/
COPY go.mod go.mod
COPY go.sum go.sum

RUN go build \
  -o bin/receiver \
  -ldflags "-w -X github.com/brigadecore/brigade-github-gateway/internal/version.version=$VERSION -X github.com/brigadecore/brigade-github-gateway/internal/version.commit=$COMMIT" \
  ./receiver

FROM scratch
COPY --from=0 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=0 /src/bin/ /brigade-github-gateway/bin/
ENTRYPOINT ["/brigade-github-gateway/bin/receiver"]
