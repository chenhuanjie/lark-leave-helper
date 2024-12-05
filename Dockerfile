FROM --platform=$BUILDPLATFORM golang:alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /app
COPY . /app
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o lark-leave-helper .

FROM alpine
RUN apk --no-cache add tzdata
COPY --from=build /app/lark-leave-helper /lark-leave-helper
ENTRYPOINT ["/lark-leave-helper"]
