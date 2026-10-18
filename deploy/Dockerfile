# syntax=docker/dockerfile:1
# Image des Render-Dienstes pdx-server. Reproduzierbar: Basisimages per
# Digest, Go ohne cgo mit -trimpath und ohne Build-ID, Zeitstempel aus
# SOURCE_DATE_EPOCH (deploy/build.sh). Zweimal gebaut ergibt denselben
# Digest.
FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.26.4-bookworm@sha256:b305420a68d0f229d91eb3b3ed9e519fcf2cf5461da4bef997bf927e8c0bfd2b AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOFLAGS=-mod=readonly \
    go build -trimpath -ldflags="-s -w -buildid=" -o /out/pdx-server ./cmd/pdx-server

# Laufzeit ohne Shell und Paketverwaltung, als Nutzer nonroot (65532).
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/pdx-server /pdx-server
USER 65532:65532
EXPOSE 8080
ENV PDX_SERVER_CONFIG=/etc/pdx/pdx-server.json
ENTRYPOINT ["/pdx-server"]
