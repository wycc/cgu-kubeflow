FROM ubuntu:22.04
RUN apt-get update && apt-get install -y \
    ca-certificates \
    mailcap \
    curl \
    jq \
    bash \
 && rm -rf /var/lib/apt/lists/*

COPY healthcheck.sh /healthcheck.sh
RUN chmod +x /healthcheck.sh  # Make the script executable

HEALTHCHECK --start-period=2s --interval=5s --timeout=3s \
    CMD /healthcheck.sh || exit 1

VOLUME /srv
EXPOSE 80

COPY docker_config.json /.filebrowser.json
COPY filebrowser /filebrowser

ENTRYPOINT [ "/filebrowser" ]