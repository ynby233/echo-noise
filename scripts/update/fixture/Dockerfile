FROM alpine:3.22
ARG REVISION
ARG VERSION
ARG BUILT_AT
LABEL org.opencontainers.image.revision=$REVISION
LABEL org.opencontainers.image.version=$VERSION org.opencontainers.image.created=$BUILT_AT
COPY coordinator /fixture
COPY update-tool /app/update-tool
COPY docker-entrypoint.sh /app/docker-entrypoint.sh
WORKDIR /app
RUN mkdir -p /data /app/config /app/data && printf 'database:\n  type: sqlite\n  path: /data/fixture.db\n' > /app/config/config.yaml && chmod +x /app/docker-entrypoint.sh /app/update-tool && ln -s /fixture /app/noise
EXPOSE 1314
HEALTHCHECK --interval=1s --timeout=3s --retries=30 CMD ["/fixture", "-health"]
ENTRYPOINT ["/fixture"]
