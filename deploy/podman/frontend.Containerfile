FROM docker.io/oven/bun:1.3.13@sha256:87416c977a612a204eb54ab9f3927023c2a3c971f4f345a01da08ea6262ae30e AS build

ARG VCS_REF
WORKDIR /src/frontend
RUN echo "$VCS_REF" | grep -Eq '^[0-9a-f]{40,64}$'

COPY frontend/package.json frontend/bun.lock ./
COPY frontend/packages/admin-app/package.json packages/admin-app/package.json
COPY frontend/packages/shared-components/package.json packages/shared-components/package.json
COPY frontend/packages/user-app/package.json packages/user-app/package.json
RUN bun install --frozen-lockfile

COPY frontend/ ./
ENV VITE_API_BASE_URL=/api/v1
RUN bun run build:user \
    && bun run --cwd packages/admin-app build -- --base=/admin/

FROM docker.io/library/nginx:1.27-alpine@sha256:65645c7bb6a0661892a8b03b89d0743208a18dd2f3f17a54ef4b76fb8e2f2a10

ARG VCS_REF
LABEL org.opencontainers.image.revision="$VCS_REF"

COPY deploy/podman/nginx.conf /etc/nginx/nginx.conf
COPY --from=build /src/frontend/packages/user-app/dist/ /usr/share/nginx/html/
COPY --from=build /src/frontend/packages/admin-app/dist/ /usr/share/nginx/html/admin/

USER nginx
EXPOSE 8080
