FROM docker.io/library/node:20-alpine
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci --omit=dev --prefer-offline --no-audit --no-fund
COPY server.js ./
COPY src/ ./src/
ENV MCP_TRANSPORT=stdio
CMD ["node","server.js"]
