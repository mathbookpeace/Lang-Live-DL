FROM amd64/ubuntu:22.10

WORKDIR /lang-live-dl-app
COPY . /lang-live-dl-app

CMD ["sh", "start.sh"]