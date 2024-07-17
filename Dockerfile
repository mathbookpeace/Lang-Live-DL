FROM amd64/ubuntu:24.04

WORKDIR /lang-live-dl-app
COPY . /lang-live-dl-app

ENV DEBIAN_FRONTEND=noninteractive

CMD ["sh", "start.sh"]