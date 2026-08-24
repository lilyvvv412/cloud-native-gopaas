FROM docker.m.daocloud.io/library/alpine:3.19
RUN apk add --no-cache ca-certificates
COPY bin/linux/resource-service /usr/local/bin/resource-service
EXPOSE 8081 9191 9091
ENTRYPOINT ["resource-service"]
