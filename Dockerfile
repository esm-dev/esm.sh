FROM golang:1.16

EXPOSE 80 

WORKDIR /

RUN apt-get update -y && apt-get install -y xz-utils

RUN git clone https://github.com/postui/esm.sh

WORKDIR /esm.sh

RUN sh ./scripts/build.sh

CMD ["./scripts/esmd", "-dev", "-domain", "localhost"]