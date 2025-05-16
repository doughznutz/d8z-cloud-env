#FROM env:base
FROM ubuntu:22.04

#RUN apt-key adv --keyserver keyserver.ubuntu.com --recv-keys E88979FB9B30ACF2
RUN apt-get update
RUN apt-get install -y \
        environment-modules

RUN apt-get install -y \
        git

RUN apt-get install -y \
        wget
        
RUN apt-get install -y \
        curl

RUN apt-get install -y \
        zip

RUN apt-get install -y \
        apt-transport-https \
        ca-certificates \
        clang \
        cmake \
        lsb-release \
        ninja-build \
        pkg-config \
        xz-utils \
        libglu1-mesa \
        libgtk-3-dev \
        libx11-6 \
        libxext6 \
        libxrender1 \
        libxtst6 \
        software-properties-common

RUN apt-get install -y \
        openjdk-17-jdk \
        lib32stdc++6 \
        libc6-i386 \
        libxtst6 \
        libxext6 \
        libxrender1 \
        libxi6 \
        x11-apps \
        xauth \
        libx11-6 \
        libx11-dev \
        libncurses6

# Install the modules environment tools.
RUN apt update && apt install -y environment-modules
RUN echo "source /usr/share/modules/init/bash" >> /etc/profile

# Copy directory into the container.
RUN chmod a+rw /root
COPY . /root/env/

# Pickup our aliases so far.
COPY ./bash_aliases bash_aliases
RUN cat bash_aliases >> ~root/.bash_aliases && rm bash_aliases

# Add a non-root user
ARG USERNAME
ARG PASSWORD
RUN useradd -m -s /bin/bash $USERNAME
RUN echo "$USERNAME:$PASSWORD" | chpasswd
RUN usermod -aG sudo $USERNAME

# Set working directory
USER $USERNAME
WORKDIR /home/$USERNAME
RUN echo "source /usr/share/modules/init/bash" >> .bashrc
#RUN cp ~root/.bash_aliases .bash_aliases

# Entrypoint for the container
USER root
COPY *.conf /etc/supervisor/conf.d/
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]