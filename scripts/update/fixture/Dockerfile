FROM scratch
ARG REVISION
LABEL org.opencontainers.image.revision=$REVISION
COPY coordinator /fixture
EXPOSE 1314
HEALTHCHECK --interval=1s --timeout=3s --retries=30 CMD ["/fixture", "-health"]
ENTRYPOINT ["/fixture"]
