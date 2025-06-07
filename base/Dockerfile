# This is the base Docker environtment which reflects the environtment on my laptop.
# It is intended to be used in the cloud, or on my new laptop that I havent bought yet.

# FROM dorowu/ubuntu-desktop-lxde-vnc
FROM ubuntu:latest

# TODO: Gather nutz from docker, github, bazel, cloud

# Install Core tools
RUN apt-get update \
    && apt-get install -y make emacs \
    && apt-get clean

RUN apt-get update \
    && apt-get install -y git \
    && apt-get clean

# Install Docker tools
RUN apt-get update \
    && apt-get install -y docker.io \
    && apt-get clean

# Install docker-compose
RUN mkdir -p /usr/libexec/docker/cli-plugins
ADD https://github.com/docker/compose/releases/download/v2.24.5/docker-compose-linux-x86_64 \
          /usr/libexec/docker/cli-plugins/docker-compose
RUN chmod +x /usr/libexec/docker/cli-plugins/docker-compose

# This is to help track the Docker hierarchy, by copying the context to /docker/self
# RUN [ -d "/docker/self" ] && mv /docker/self /docker/self/parent || true
COPY . /docker/self

# Add a non-root User.
#ARG USERNAME
#ARG PASSWORD
#RUN id -u $USERNAME 2>/dev/null || useradd -m -s /bin/bash $USERNAME
#RUN echo "$USERNAME:$PASSWORD" | chpasswd
#RUN usermod -aG sudo $USERNAME

# Entrypoint for the container
#COPY *.conf /etc/supervisor/conf.d/
#RUN sed -i 's/^chmod=0[0-9][0-9][0-9]/chmod=0777/' /etc/supervisor/supervisord.conf
#CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]
CMD ["/bin/bash"]
#ENTRYPOINT ["entrypoint.sh"]