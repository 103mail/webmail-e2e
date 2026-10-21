# The suite runs in a container on the stack's federation network, so it
# reaches every Domain by its real Federation hostname through Docker's DNS,
# the way the Domains reach each other, with no /etc/hosts entries. It drives
# the stack's controls with the docker CLI, so the image carries it and the
# compose plugin. See run.sh.
FROM docker:29-cli AS docker

FROM golang:1.26
COPY --from=docker /usr/local/bin/docker /usr/local/bin/docker
COPY --from=docker /usr/local/libexec/docker/cli-plugins/docker-compose /usr/local/libexec/docker/cli-plugins/docker-compose
# run.sh runs the suite as the calling user, so the files it writes into the
# deploy directory stay theirs. The caches live in a volume any user can write.
RUN mkdir -p /cache && chmod 1777 /cache
ENV HOME=/cache GOCACHE=/cache/build GOMODCACHE=/cache/mod
