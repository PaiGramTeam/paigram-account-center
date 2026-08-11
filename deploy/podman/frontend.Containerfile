FROM docker.io/oven/bun:1.3.13 AS build

WORKDIR /src/frontend

COPY frontend/package.json frontend/bun.lock ./
COPY frontend/packages/admin-app/package.json packages/admin-app/package.json
COPY frontend/packages/shared-components/package.json packages/shared-components/package.json
COPY frontend/packages/user-app/package.json packages/user-app/package.json
RUN bun install --frozen-lockfile

COPY frontend/ ./
ENV VITE_API_BASE_URL=/api/v1
RUN bun run build:user \
    && bun run --cwd packages/admin-app build -- --base=/admin/

FROM docker.io/library/nginx:1.27-alpine

COPY deploy/podman/nginx.conf /etc/nginx/nginx.conf
COPY --from=build /src/frontend/packages/user-app/dist/ /usr/share/nginx/html/
COPY --from=build /src/frontend/packages/admin-app/dist/ /usr/share/nginx/html/admin/

USER nginx
EXPOSE 8080
