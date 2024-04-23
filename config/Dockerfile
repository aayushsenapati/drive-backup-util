FROM alpine:latest

RUN apk add --update git && \
    git config --global user.email "aayuanku@gmail.com" && \
    git config --global user.name "Aayush Nagar"

WORKDIR /app

COPY quickstart-linux .

CMD ["/app/quickstart-linux"]