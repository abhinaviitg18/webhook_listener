FROM node:20-bookworm-slim AS web-build
WORKDIR /app/web

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

FROM golang:1.24-bookworm AS go-build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=web-build /app/web/dist/ /app/internal/ui/dist/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM gcr.io/distroless/base-debian12

WORKDIR /app
COPY --from=go-build /out/server /app/server

EXPOSE 8080

CMD ["/app/server"]
