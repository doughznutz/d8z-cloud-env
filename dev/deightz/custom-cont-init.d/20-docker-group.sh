#!/bin/bash
# Let abc talk to the mounted host docker.sock without sudo (DinD support).
if [ -S /var/run/docker.sock ]; then
    SOCK_GID=$(stat -c '%g' /var/run/docker.sock)
    GROUP_NAME=$(getent group "$SOCK_GID" | cut -d: -f1)
    if [ -z "$GROUP_NAME" ]; then
        groupadd -g "$SOCK_GID" docker-host
        GROUP_NAME=docker-host
    fi
    usermod -aG "$GROUP_NAME" abc
fi
