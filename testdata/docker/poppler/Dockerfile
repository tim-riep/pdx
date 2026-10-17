# Poppler zum Rastern und zur Textextraktion, zbar zum Dekodieren von
# Barcodes in Vergleichstests. Nur für Tests.
#   docker build -t pdx-test/poppler:2 testdata/docker/poppler
FROM debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
RUN apt-get update \
 && apt-get install -y --no-install-recommends poppler-utils zbar-tools \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /work
