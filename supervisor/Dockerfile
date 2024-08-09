ARG BUILD_FROM
FROM ${BUILD_FROM}

COPY ./patch /patch

RUN apk add --no-cache patch && \
    cd /usr/src/supervisor/ && \
    for patch in /patch/*.patch; do \
        patch -p1 < "$patch"; \
    done && \
    python3 -m compileall /usr/src/supervisor/supervisor
