# linejudge API
FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY fixtures ./fixtures
RUN CGO_ENABLED=0 go build -o /out/linejudge ./cmd/linejudge && \
    CGO_ENABLED=0 go build -o /out/linejudge-eval ./cmd/eval

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/linejudge /app/linejudge
COPY --from=build /out/linejudge-eval /app/linejudge-eval
COPY fixtures /app/fixtures
ENV LINEJUDGE_FIXTURES=/app/fixtures
ENV LINEJUDGE_DB=/data/linejudge.db
ENV LINEJUDGE_ADDR=:8080
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/app/linejudge"]
