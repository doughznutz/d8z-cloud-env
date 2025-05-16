#
# FILE: ~/docker/tools/flutter/Dockerfile

# Dont use a custom image as fetcher or you have to rebuild when it is.
FROM ubuntu:latest AS fetcher

# Install required dependencies
RUN apt-get update
RUN apt-get install -y curl gnupg

# Add Google Cloud SDK repository - setup pubkey
RUN curl https://packages.cloud.google.com/apt/doc/apt-key.gpg | gpg --dearmor -o /usr/share/keyrings/cloud.google.gpg

# Add Google Cloud SDK repository (for Dart SDK dependency)
RUN echo "deb [signed-by=/usr/share/keyrings/cloud.google.gpg] http://packages.cloud.google.com/apt cloud-sdk main" \
    | tee -a /etc/apt/sources.list.d/google-cloud-sdk.list


# Download Flutter SDK
RUN curl -o /tmp/flutter_linux_3.27.3-stable.tar.xz \
    https://storage.googleapis.com/flutter_infra_release/releases/stable/linux/flutter_linux_3.27.3-stable.tar.xz


# Start a new container and copy the files fetched above.
FROM tools:base
COPY --from=fetcher /usr/share/keyrings/cloud.google.gpg /usr/share/keyrings/cloud.google.gpg
COPY --from=fetcher /etc/apt/sources.list.d/google-cloud-sdk.list /etc/apt/sources.list.d/google-cloud-sdk.list 
COPY --from=fetcher /tmp/flutter_linux_3.27.3-stable.tar.xz /tmp/flutter_linux_3.27.3-stable.tar.xz


# Update and install Dart SDK
# TODO: Do we need this in vscode container?
RUN apt-get update && apt-get install -y google-cloud-sdk

# Set Dart SDK path
ENV PATH="$PATH:/usr/lib/google-cloud-sdk/platform/google_appengine/goroot/bin"

# Unzip Flutter SDK
RUN mkdir /opt/flutter
RUN tar -xf /tmp/flutter_linux_3.27.3-stable.tar.xz -C /opt
RUN chmod -R a+w /opt/flutter

# Clean up apt cache to reduce image size
RUN apt-get clean && rm -rf /var/lib/apt/lists/*

# Load the environment with: module load flutter
COPY . /root/env

# TODO: If you want to start flutter, add a .conf file.

# Entrypoint for the container
COPY *.conf /etc/supervisor/conf.d/
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]