# Traefik front door for the dashboard console: file-provider routing + basic auth.
# busybox-extras supplies httpd, which serves the console shell page at `/`.
FROM traefik:v3.1

RUN apk add --no-cache openssl busybox-extras

COPY traefik.yml /etc/traefik/traefik.yml
COPY dynamic.yml /etc/traefik/dynamic.yml
COPY console/ /srv/console/
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
