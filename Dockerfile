# Build stage
FROM golang:1.22-alpine AS build

WORKDIR /src

# Dependency proxy is overridable for restricted networks, e.g.
#   docker build --build-arg GOPROXY=https://goproxy.cn,direct .
ARG GOPROXY=https://proxy.golang.org,direct

# Cache dependencies independently of source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/cyclone-service .

# Runtime stage
FROM alpine:3.20

RUN adduser -D -u 10001 appuser
COPY --from=build /out/cyclone-service /usr/local/bin/cyclone-service
USER appuser

EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/cyclone-service"]
