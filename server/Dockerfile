FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/agent-message-bus . && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/agent-message-bus /agent-message-bus
COPY --from=build --chown=65532:65532 /out/data /data
VOLUME /data
# The web GUI is off in the image; publish it with -e AMB_WEB_ADDR=:8081 -p 8081:8081 (DEPLOYMENT.md §6).
EXPOSE 8080
ENV AMB_ADDR=:8080 AMB_WEB_ADDR=off AMB_DB_PATH=/data/agent-message-bus.db
USER nonroot
ENTRYPOINT ["/agent-message-bus"]
