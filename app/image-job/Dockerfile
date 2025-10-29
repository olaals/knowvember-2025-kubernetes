FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
# static binary (no CGO, no external deps)
RUN CGO_ENABLED=0 go build -o job

# Runtime stage
FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /src/job /app/job
USER nonroot:nonroot
ENTRYPOINT ["/app/job"]