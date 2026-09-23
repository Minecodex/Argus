# syntax=docker/dockerfile:1.7
FROM node:24.19.0-alpine AS build

ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
WORKDIR /src

RUN corepack enable && corepack prepare pnpm@11.21.0 --activate
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml tsconfig.base.json ./
COPY scripts/build-web.mjs scripts/check-real-build.mjs ./scripts/
COPY web ./web
RUN --mount=type=cache,id=argus-pnpm-store,target=/pnpm/store \
    pnpm install --frozen-lockfile --fetch-retries 5 --fetch-timeout 300000
ARG VITE_API_MODE=real
ARG VITE_API_BASE_URL=/
# Template origin and platform login URL are resolved at runtime from
# /argus-runtime.json (injected by Helm); VITE_* values are dev/mock overrides.
ARG VITE_TEMPLATE_ORIGIN=
ARG VITE_PLATFORM_URL=
ARG VITE_DIRECT_EGRESS_ADDRESSES=
RUN VITE_API_MODE=$VITE_API_MODE \
    VITE_API_BASE_URL=$VITE_API_BASE_URL \
    VITE_TEMPLATE_ORIGIN=$VITE_TEMPLATE_ORIGIN \
    VITE_PLATFORM_URL=$VITE_PLATFORM_URL \
    VITE_DIRECT_EGRESS_ADDRESSES=$VITE_DIRECT_EGRESS_ADDRESSES \
    node scripts/build-web.mjs "$VITE_API_MODE"

FROM nginxinc/nginx-unprivileged:1.29.4-alpine
COPY deploy/docker/nginx.conf /etc/nginx/nginx.conf
COPY --from=build /src/web/apps/enterprise/dist /srv/enterprise
COPY --from=build /src/web/apps/platform/dist /srv/platform
COPY --from=build /src/web/apps/template-runtime/dist /srv/template-runtime
EXPOSE 8080 8081 8083
