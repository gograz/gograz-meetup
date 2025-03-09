FROM golang:1.24.1 AS builder
COPY . /src
WORKDIR /src
RUN go mod download
RUN CGO_ENABLED=0 go build -o gograz-meetup

FROM gcr.io/distroless/static-debian12:latest
COPY --from=builder /src/gograz-meetup /bin/gograz-meetup
ENTRYPOINT ["/bin/gograz-meetup"]
