FROM golang:1.21.0-alpine as build

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
WORKDIR cmd
RUN go build -v -o /usr/local/bin/mocrelay ./...

FROM gcr.io/distroless/static-debian12
COPY --from=build /usr/local/bin/mocrelay /

EXPOSE 8234

CMD ["/mocrelay"]
