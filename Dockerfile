FROM alpine
RUN apk add --update docker
ADD logspout-restarter /
ENTRYPOINT ["/logspout-restarter"]
