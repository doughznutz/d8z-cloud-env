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

# Setup the VNC and terminal.
ENV RESOLUTION=1920x1080

# Copy directory into the container.
COPY . /docker/self

# Pickup our aliases so far.
COPY ./bash_aliases bash_aliases
RUN cat bash_aliases >> ~root/.bash_aliases && rm bash_aliases


# Add a non-root User.
ARG USERNAME
ARG PASSWORD
RUN useradd -m -s /bin/bash $USERNAME
RUN echo "$USERNAME:$PASSWORD" | chpasswd
RUN usermod -aG sudo $USERNAME

# Change to that user??
#USER $USERNAME

# Entrypoint for the container
COPY *.conf /etc/supervisor/conf.d/
RUN sed -i 's/^chmod=0[0-9][0-9][0-9]/chmod=0777/' /etc/supervisor/supervisord.conf
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]