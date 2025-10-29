FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server .

FROM gcr.io/distroless/static-debian12
ENV PORT=8045 \
    UPSTREAM_API=http://api:8045
WORKDIR /app
COPY --from=build /app/server /app/server
USER nonroot:nonroot
EXPOSE 8045
ENTRYPOINT ["/app/server"]
