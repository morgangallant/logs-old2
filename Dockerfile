# Build step.
FROM golang:1.16 as build
ADD . /src
WORKDIR /src/
RUN go build -o server logs/server.go

# Run Step using Distroless.
FROM gcr.io/distroless/base
WORKDIR /mg
COPY --from=build /src/server /mg/
ENTRYPOINT [ "/mg/server" ]
