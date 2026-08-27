FROM nginx:1.27-alpine

ARG VCS_REF
ARG CONTRACT_BASELINE
ARG SDK_VERSION
RUN echo "$VCS_REF" | grep -Eq '^[0-9a-f]{40,64}$' \
    && echo "$CONTRACT_BASELINE" | grep -Eq '^[0-9a-f]{40,64}$' \
    && echo "$SDK_VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$'
LABEL org.opencontainers.image.revision="$VCS_REF" \
    org.paigram.contract-baseline="$CONTRACT_BASELINE" \
    org.paigram.sdk-version="$SDK_VERSION"

COPY nginx.conf /etc/nginx/nginx.conf
COPY user/ /usr/share/nginx/html/
COPY admin/ /usr/share/nginx/html/admin/

USER nginx
EXPOSE 8080
