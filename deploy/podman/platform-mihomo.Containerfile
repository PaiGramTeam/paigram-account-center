FROM docker.io/library/golang:1.25.7-alpine@sha256:f6751d823c26342f9506c03797d2527668d095b0a15f1862cddb4d927a7a4ced AS build

ARG VCS_REF
WORKDIR /src
RUN echo "$VCS_REF" | grep -Eq '^[0-9a-f]{40,64}$'

COPY go.work go.work.sum ./
COPY contracts/gen/go/go.mod contracts/gen/go/go.sum contracts/gen/go/
COPY contracts/runtime/go/go.mod contracts/runtime/go/go.sum contracts/runtime/go/
COPY services/platform-mihomo/go.mod services/platform-mihomo/go.sum services/platform-mihomo/
RUN cd services/platform-mihomo && go mod download

COPY contracts/gen/go/ contracts/gen/go/
COPY contracts/runtime/go/ contracts/runtime/go/
COPY services/platform-mihomo/ services/platform-mihomo/
RUN cd services/platform-mihomo \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/platform-mihomo ./cmd/platform-mihomo-service \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/platform-mihomo-healthcheck ./cmd/platform-mihomo-healthcheck

FROM docker.io/library/alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40

ARG VCS_REF
LABEL org.opencontainers.image.revision="$VCS_REF"

RUN addgroup -S -g 10002 platform \
    && adduser -S -D -H -u 10002 -G platform platform

COPY --from=build /out/platform-mihomo /usr/local/bin/platform-mihomo
COPY --from=build /out/platform-mihomo-healthcheck /usr/local/bin/platform-mihomo-healthcheck
COPY --chown=platform:platform services/platform-mihomo/initialize/migrate/sql/ /opt/platform-mihomo/initialize/migrate/sql/
COPY --chown=platform:platform deploy/podman/platform-mihomo.config.yaml /opt/platform-mihomo/config/config.yaml

USER platform
WORKDIR /opt/platform-mihomo

EXPOSE 9000 9001
ENTRYPOINT ["/usr/local/bin/platform-mihomo"]
CMD ["-conf", "/opt/platform-mihomo/config/config.yaml"]
