# Postgres for trackid: pgvector (face embeddings) and PostGIS (occurrence locations). No
# official image ships both, so PostGIS is added to pgvector's from the PGDG apt repository
# the base image already uses.
FROM pgvector/pgvector:pg16
RUN apt-get update \
    && apt-get install -y --no-install-recommends postgresql-16-postgis-3 \
    && rm -rf /var/lib/apt/lists/*
