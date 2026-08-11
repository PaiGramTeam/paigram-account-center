FROM docker.io/library/golang:1.25.7-alpine AS build

WORKDIR /src

COPY go.work go.work.sum ./
COPY contracts/gen/go/go.mod contracts/gen/go/go.sum contracts/gen/go/
COPY services/account-center/go.mod services/account-center/go.sum services/account-center/
COPY services/platform-mihomo/go.mod services/platform-mihomo/go.sum services/platform-mihomo/
RUN cd services/account-center && go mod download

COPY contracts/gen/go/ contracts/gen/go/
COPY services/account-center/ services/account-center/
RUN cd services/account-center \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/paigram ./cmd/paigram

FROM docker.io/library/alpine:3.23

RUN addgroup -S -g 10001 paigram \
    && adduser -S -D -H -u 10001 -G paigram paigram

COPY --from=build /out/paigram /usr/local/bin/paigram
COPY --chown=paigram:paigram services/account-center/initialize/migrate/sql/ /opt/paigram/migrations/
COPY --chown=paigram:paigram deploy/podman/config.yaml /opt/paigram/config/config.yaml

USER paigram
WORKDIR /opt/paigram

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/paigram"]
CMD ["serve"]
