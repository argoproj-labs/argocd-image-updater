FROM nginx:alpine-slim

RUN set -x && \
  apk --update upgrade                                  &&  \
  apk add git git-daemon bash fcgiwrap spawn-fcgi wget openssl apache2-utils  &&  \
  adduser git -h /var/lib/git -D                        &&  \
  addgroup nginx git                                     &&  \
  git config --system http.receivepack true             &&  \
  git config --system http.uploadpack true              &&  \
  git config --system user.email "gitserver@git.com"    &&  \
  git config --system user.name "Git Server"            &&  \
  ln -sf /dev/stdout /var/log/nginx/access.log          &&  \
  ln -sf /dev/stderr /var/log/nginx/error.log

COPY ./etc /etc
COPY ./entrypoint.sh /usr/local/bin/entrypoint

ENTRYPOINT [ "entrypoint" ]
CMD [ "-start" ]

COPY ./testdata /var/lib/initial/testdata
