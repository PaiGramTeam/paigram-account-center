FROM docker.io/library/alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40 AS metadata

ARG VCS_REF
ARG CONTRACT_BASELINE
ARG SDK_VERSION
RUN echo "$VCS_REF" | grep -Eq '^[0-9a-f]{40,64}$' \
    && echo "$CONTRACT_BASELINE" | grep -Eq '^[0-9a-f]{40,64}$' \
    && echo "$SDK_VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$'

FROM docker.io/library/golang:1.25.7-alpine@sha256:f6751d823c26342f9506c03797d2527668d095b0a15f1862cddb4d927a7a4ced AS build

WORKDIR /src

COPY go.work go.work.sum ./
COPY contracts/gen/go/go.mod contracts/gen/go/go.sum contracts/gen/go/
COPY contracts/runtime/go/go.mod contracts/runtime/go/go.sum contracts/runtime/go/
COPY services/account-center/go.mod services/account-center/go.sum services/account-center/
COPY services/platform-mihomo/go.mod services/platform-mihomo/go.sum services/platform-mihomo/
RUN cd services/account-center && go mod download

COPY contracts/gen/go/ contracts/gen/go/
COPY contracts/runtime/go/ contracts/runtime/go/
COPY services/account-center/ services/account-center/
RUN cd services/account-center \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/paigram ./cmd/paigram \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/recovery-dsn-verify ./cmd/recovery-dsn-verify

FROM metadata

ARG VCS_REF
ARG CONTRACT_BASELINE
ARG SDK_VERSION
LABEL org.opencontainers.image.revision="$VCS_REF" \
    org.paigram.contract-baseline="$CONTRACT_BASELINE" \
    org.paigram.sdk-version="$SDK_VERSION"

RUN addgroup -S -g 10001 paigram \
    && adduser -S -D -H -u 10001 -G paigram paigram

COPY --from=build /out/paigram /usr/local/bin/paigram
COPY --from=build /out/recovery-dsn-verify /usr/local/bin/recovery-dsn-verify
COPY --chown=paigram:paigram services/account-center/initialize/migrate/sql/ /opt/paigram/migrations/
COPY --chown=paigram:paigram deploy/podman/config.yaml /opt/paigram/config/config.yaml

USER paigram
WORKDIR /opt/paigram

EXPOSE 8080 50051
ENTRYPOINT ["/usr/local/bin/paigram"]
CMD ["serve"]
