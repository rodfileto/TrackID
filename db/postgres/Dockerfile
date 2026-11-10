# Postgres for trackid: pgvector (face embeddings) and PostGIS (occurrence locations). No
# official image ships both, so PostGIS is added to pgvector's from the PGDG apt repository
# the base image already uses.
#
# gosu, which the image's entrypoint uses to drop from root to postgres, is rebuilt with a
# current Go: the base image's build carries an old Go standard library, which the release image
# scan reports. The same release, from source.
FROM golang:1.27.3-bookworm AS gosu
ARG GOSU_VERSION=1.20
RUN git clone --depth 1 --branch "${GOSU_VERSION}" https://github.com/tianon/gosu.git /src \
    && cd /src && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /gosu .

FROM pgvector/pgvector:0.8.7-pg16-bookworm
# Debian's security fixes released since the base image was cut, then PostGIS.
RUN apt-get update \
    && apt-get upgrade -y --no-install-recommends \
    && apt-get install -y --no-install-recommends postgresql-16-postgis-3 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=gosu /gosu /usr/local/bin/gosu
RUN gosu nobody true
