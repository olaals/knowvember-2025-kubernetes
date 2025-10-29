FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o api

FROM gcr.io/distroless/base-debian12
ENV PORT=8050
WORKDIR /app
COPY --from=build /app/api /app/api
COPY job.yaml /app/job.yaml
USER nonroot:nonroot
EXPOSE 8050
ENTRYPOINT ["/app/api"]
