ARG GO_VERSION=1.27.3

FROM golang:${GO_VERSION} AS deps
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

FROM deps AS build
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

FROM gcr.io/distroless/static-debian12:nonroot AS api
COPY --from=build /out/api /api
COPY --chown=nonroot:nonroot samples /samples
EXPOSE 3000
ENTRYPOINT ["/api"]

FROM gcr.io/distroless/static-debian12:nonroot AS migrate
COPY --from=build /out/migrate /migrate
ENTRYPOINT ["/migrate"]

FROM gcr.io/distroless/static-debian12:nonroot AS seed
COPY --from=build /out/seed /seed
ENTRYPOINT ["/seed"]

FROM gcr.io/distroless/static-debian12:nonroot AS payment-mock
COPY --from=build /out/payment-mock /payment-mock
EXPOSE 4000
ENTRYPOINT ["/payment-mock"]
