ARG BASE
FROM ${BASE}
COPY identity-mismatch.sh /app/noise
RUN chmod +x /app/noise
HEALTHCHECK --interval=1s --timeout=2s --start-period=1s CMD true
ENTRYPOINT ["/app/noise"]
CMD []
