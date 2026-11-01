# syntax=docker/dockerfile:1
# Image of the render service pdx-server. Reproducible: base images by
# digest, Go without cgo with -trimpath and without build ID, timestamps
# from SOURCE_DATE_EPOCH (deploy/build.sh). Building twice yields the same
# digest.
FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.26.10-bookworm@sha256:23ce71337b726608afb1bc00986e630a05034a87a87ea1fc2cd1ae5cf33387eb AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOFLAGS=-mod=readonly \
    go build -trimpath -ldflags="-s -w -buildid=" -o /out/pdx-server ./cmd/pdx-server

# Runtime without shell and package manager, as user nonroot (65532).
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/pdx-server /pdx-server
COPY --from=build /src/LICENSE /src/NOTICE /src/THIRD_PARTY_NOTICES.txt /usr/share/doc/pdx/
USER 65532:65532
EXPOSE 8080
ENV PDX_SERVER_CONFIG=/etc/pdx/pdx-server.json
ENTRYPOINT ["/pdx-server"]
