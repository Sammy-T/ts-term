# syntax=docker/dockerfile:1

ARG NODE_VERSION=22.13.1
ARG PNPM_VERSION=10.14.0
ARG GO_VERSION=1.24.6

## Node
################################################################################
# Use node image for base image
FROM node:${NODE_VERSION}-alpine AS base-node
WORKDIR /app/web

# Install pnpm.
RUN --mount=type=cache,target=/root/.npm \
    npm install -g pnpm@${PNPM_VERSION}

################################################################################
# Create a stage for installing production dependecies.
FROM base-node AS deps-node

# Download dependencies as a separate step to take advantage of Docker's caching.
# Leverage a cache mount to /root/.local/share/pnpm/store to speed up subsequent builds.
# Leverage bind mounts to package.json and pnpm-lock.yaml to avoid having to copy them
# into this layer.
RUN --mount=type=bind,source=web/package.json,target=package.json \
    --mount=type=bind,source=web/pnpm-lock.yaml,target=pnpm-lock.yaml \
    --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --prod --frozen-lockfile

################################################################################
# Create a stage for building the application.
FROM deps-node AS build-node

# Download additional development dependencies before building, as some projects require
# "devDependencies" to be installed to build. If you don't need this, remove this step.
RUN --mount=type=bind,source=web/package.json,target=package.json \
    --mount=type=bind,source=web/pnpm-lock.yaml,target=pnpm-lock.yaml \
    --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

# Copy the rest of the source files into the image.
COPY web .
# Run the build script.
RUN pnpm run build


## Go
################################################################################
# Create a stage for building the application.
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS build-go
WORKDIR /app

# Download dependencies as a separate step to take advantage of Docker's caching.
# Leverage a cache mount to /go/pkg/mod/ to speed up subsequent builds.
# Leverage bind mounts to go.sum and go.mod to avoid having to copy them into
# the container.
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    go mod download -x

# This is the architecture you're building for, which is passed in by the builder.
# Placing it here allows the previous steps to be cached across architectures.
ARG TARGETARCH

# Build the application.
# Leverage a cache mount to /go/pkg/mod/ to speed up subsequent builds.
# Leverage a bind mount to the current directory to avoid having to copy the
# source code into the container.
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOARCH=$TARGETARCH go build -o /bin/server .


## App
################################################################################
# Create a new stage for running the application that contains the minimal
# runtime dependencies for the application. This often uses a different base
# image from the build stage where the necessary files are copied from the build
# stage.
FROM alpine:latest AS final
WORKDIR /app

# Configure terminal color
ENV TERM=xterm-256color
ENV PS1="\e[92m\u\e[0m@\e[94m\h\e[0m:\e[35m\w\e[0m$ "

# Install any runtime dependencies that are needed to run your application.
# Leverage a cache mount to /var/cache/apk/ to speed up subsequent builds.
RUN --mount=type=cache,target=/var/cache/apk \
    apk --update add \
        ca-certificates \
        tzdata \
        bash \
        curl \
        openssh-client \
        && \
        update-ca-certificates

# Create a non-privileged user that the app will run under.
# See https://docs.docker.com/go/dockerfile-user-best-practices/
ARG UID=10001
RUN adduser \
    --disabled-password \
    --gecos "" \
    --shell "/sbin/nologin" \
    --uid "${UID}" \
    appuser
USER appuser

# Create the ssh directory. (It can optionally be persisted to volume.)
RUN mkdir /home/appuser/.ssh

# Create the known_hosts file
RUN touch /home/appuser/.ssh/known_hosts

COPY web/package.json web/

# Copy the built node application from the "build-node" stage into the image.
COPY --from=build-node /app/web/dist web/dist/

# Copy the executable from the "build-go" stage.
COPY --from=build-go /bin/server .

# Expose the port that the application listens on.
EXPOSE 3000

# What the container should run when it is started.
ENTRYPOINT [ "/app/server" ]
