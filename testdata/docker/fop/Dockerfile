# Apache FOP als Referenz für Vergleichstests. Nur für Tests, nie zur Laufzeit.
# Bauen aus der Wurzel des Repos (Kontext), damit die Liberation-Schriften
# aus font/fonts/liberation kopiert werden können:
#   docker build -t pdx-test/fop:2.11 -f testdata/docker/fop/Dockerfile .
FROM eclipse-temurin:21-jre@sha256:cff19e6215689161eb6162c11b86b0c60ddf802164f2eaf48d570f8fb79a36c5

ARG FOP_VERSION=2.11
ARG FOP_SHA512=47f26517cdb514415e584a804763a5b81b8f178279d921c73b9c0a6a179f3cfc13ac2071a2085277158b336ad9c84e40a86358aa235da82a8f7ef4e83b994b16

RUN curl -fsSL -o /tmp/fop.tgz "https://archive.apache.org/dist/xmlgraphics/fop/binaries/fop-${FOP_VERSION}-bin.tar.gz" \
 && echo "${FOP_SHA512}  /tmp/fop.tgz" | sha512sum -c - \
 && tar xzf /tmp/fop.tgz -C /opt \
 && rm /tmp/fop.tgz \
 && ln -s "/opt/fop-${FOP_VERSION}/fop/fop" /usr/local/bin/fop

COPY font/fonts/liberation/*.ttf /fonts/
COPY testdata/docker/fop/fop.xconf /etc/fop/fop.xconf

WORKDIR /work
ENTRYPOINT ["fop", "-c", "/etc/fop/fop.xconf"]
