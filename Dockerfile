FROM node:26-alpine AS web
WORKDIR /src/apps/web
COPY apps/web/package*.json ./
RUN npm ci
COPY apps/web/ ./
RUN npm run build

FROM golang:1.27.3-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /certree ./apps/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* && useradd --uid 10001 --create-home certree && mkdir -p /data /app/web && chown -R 10001:10001 /data /app
WORKDIR /app
COPY --from=build /certree /usr/local/bin/certree
COPY --from=web /src/apps/web/dist /app/web
COPY LICENSE NOTICE docs/THIRD_PARTY_NOTICES.md /usr/share/doc/certree/
COPY docs/licenses/ /usr/share/doc/certree/licenses/
USER 10001:10001
ENV CERTREE_ADDRESS=0.0.0.0:8080 CERTREE_DB=/data/certree.db CERTREE_WEB_DIR=/app/web
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD ["certree","-healthcheck"]
ENTRYPOINT ["certree"]
