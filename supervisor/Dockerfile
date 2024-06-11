ARG BUILD_FROM
FROM ${BUILD_FROM}

COPY ./patch /patch

RUN cd /usr/src/supervisor/ && \
    patch -p1 < /patch/*.patch && \
    python3 -m compileall /usr/src/supervisor/supervisor