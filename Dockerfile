FROM golang:1.26.0-alpine3.23 AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o main main.go types.go db.go

FROM alpine:3.23

WORKDIR /app

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/main .

CMD [ "/app/main" ]
