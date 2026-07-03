FROM golang:1.25-alpine AS build

WORKDIR /src

COPY backend/go.mod backend/go.sum ./backend/
WORKDIR /src/backend
RUN go mod download

COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/access-workspace ./cmd/server

FROM alpine:3.21

WORKDIR /app
COPY --from=build /out/access-workspace /app/access-workspace

EXPOSE 8080

CMD ["/app/access-workspace"]
