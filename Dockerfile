# This image wraps the exact Linux artifact produced by scripts/release.ps1.
FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=unknown
LABEL org.opencontainers.image.title="tinyrill" \
      org.opencontainers.image.source="https://github.com/WhiteGreeny/tinyrill" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"
COPY --chown=65532:65532 --chmod=0555 dist/tinyrill-linux-${TARGETARCH}/tinyrill /usr/local/bin/tinyrill
COPY LICENSE /usr/share/doc/tinyrill/LICENSE
USER 65532:65532
WORKDIR /state
EXPOSE 1080 1081
ENTRYPOINT ["/usr/local/bin/tinyrill"]
CMD ["run", "--config", "/run/tinyrill/customer.yaml"]
