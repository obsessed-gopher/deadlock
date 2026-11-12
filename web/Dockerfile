# syntax=docker/dockerfile:1

# Mock API over captured fixtures (docker-compose.dev.yml). Node runs the
# TypeScript directly; it imports only types from src/, which are erased.
FROM node:26-alpine AS mock
WORKDIR /app
COPY mock ./mock
USER node
ENV MOCK_PORT=8090
EXPOSE 8090
HEALTHCHECK --interval=2s --timeout=3s --retries=30 CMD wget -qO- http://127.0.0.1:8090/healthz || exit 1
CMD ["node", "mock/server.ts"]

FROM node:26-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .
RUN npm run build

# "standalone" output carries only the node_modules the server needs.
FROM node:26-alpine AS app
WORKDIR /app
ENV NODE_ENV=production PORT=3000 HOSTNAME=0.0.0.0
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
USER node
EXPOSE 3000
HEALTHCHECK --interval=5s --timeout=3s --retries=20 CMD wget -qO- http://127.0.0.1:3000/ >/dev/null || exit 1
CMD ["node", "server.js"]
