# FILE: env/base/Dockerfile

# Use dorowu/ubuntu-desktop-lxde-vnc as the base image
#> docker pull dorowu/ubuntu-desktop-lxde-vnc
FROM dorowu/ubuntu-desktop-lxde-vnc

# Install Base Environment tools
RUN apt-key adv --keyserver keyserver.ubuntu.com --recv-keys E88979FB9B30ACF2
RUN apt-get update && apt-get install -y \
    emacs \
    make \
    xsel \
    xterm \
    && apt-get clean

# Expose the default VNC and NoVNC ports
EXPOSE 5900 6080

# Setup the VNC dimensions...override in docker-compose.yaml
ENV RESOLUTION=1920x1080

# This Copy ourselves into the container at /docker/self
COPY . /docker/self

# Install Docker tools
RUN apt-get update \
    && apt-get install -y docker.io \
    && apt-get clean

# Install docker-compose
RUN mkdir -p /usr/libexec/docker/cli-plugins
ADD https://github.com/docker/compose/releases/download/v2.24.5/docker-compose-linux-x86_64 \
          /usr/libexec/docker/cli-plugins/docker-compose
RUN chmod +x /usr/libexec/docker/cli-plugins/docker-compose

# Install Node.js (using nodesource repository)
RUN curl -sL https://deb.nodesource.com/setup_18.x | bash -
RUN apt-get install -y nodejs

# Install Gemini CLI globally
RUN npm install -g @google/gemini-cli


# Add a non-root User.
ARG USERNAME
ARG PASSWORD
RUN id -u $USERNAME 2>/dev/null || useradd -m -s /bin/bash $USERNAME
RUN echo "$USERNAME:$PASSWORD" | chpasswd
RUN usermod -aG sudo $USERNAME

# Change to that user??
#USER $USERNAME

# Entrypoint for the container
COPY *.conf /etc/supervisor/conf.d/
RUN sed -i 's/^chmod=0[0-9][0-9][0-9]/chmod=0777/' /etc/supervisor/supervisord.conf
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]