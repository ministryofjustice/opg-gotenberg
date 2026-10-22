FROM gotenberg/gotenberg:8.38.1-chromium@sha256:5da4ab7b8fe7326537369e17b661be765d9a2887f77722faacdbe6d55812fb96 AS gotenberg-base

FROM gotenberg-base AS patch
USER root
RUN set -eux; \
    apt-get update; \
    apt-get install -y --no-install-recommends --only-upgrade \
        chromium chromium-common zlib1g libminizip1t64 libxml2 libexpat1 libsasl2-modules-db libcups2t64 libattr1 libacl1 \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*

FROM gotenberg-base AS production
COPY --from=patch /usr /usr
COPY --from=patch /var/lib/dpkg /var/lib/dpkg
