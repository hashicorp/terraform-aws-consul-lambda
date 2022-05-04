FROM golang:1.18-alpine as build

ENV CGO_ENABLED 0
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /bin/lambda-registrator

FROM public.ecr.aws/lambda/provided:al2
COPY --from=build /bin/lambda-registrator /bin/lambda-registrator
ENTRYPOINT [ "/bin/lambda-registrator" ]
