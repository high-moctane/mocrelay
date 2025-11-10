FROM golang:1.25.4 as build

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
WORKDIR cmd
RUN CGO_ENABLED=0 go build -v -o /usr/local/bin/mocrelay ./...

FROM gcr.io/distroless/static-debian12
COPY --from=build /usr/local/bin/mocrelay /

EXPOSE 8234

CMD ["/mocrelay"]
