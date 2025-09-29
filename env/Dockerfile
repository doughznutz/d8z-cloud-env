FROM ubuntu:latest

# Install Core tools
RUN apt-get update \
    && apt-get install -y curl dos2unix emacs jq make \
    && apt-get clean

# Install Git - Do we want GH?
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

# Download and install the Google Cloud SDK
RUN mkdir -p /usr/local/gcloud
ADD https://dl.google.com/dl/cloudsdk/release/google-cloud-sdk.tar.gz /usr/local/gcloud
RUN tar -xzf /usr/local/gcloud/google-cloud-sdk.tar.gz -C /usr/local/gcloud && rm /usr/local/gcloud/google-cloud-sdk.tar.gz
RUN /usr/local/gcloud/google-cloud-sdk/install.sh
ENV PATH $PATH:/usr/local/gcloud/google-cloud-sdk/bin

# Clean up unnecessary packages
RUN apt-get autoremove -y && apt-get clean
RUN rm -rf /var/lib/apt/lists/*

# Create a working directory
WORKDIR /self

# We will build the dev containers from here.
COPY . /self
RUN find /self -name *.sh -exec dos2unix {} +
RUN find /self -name *.sh -exec chmod +x {} +

# Set the entrypoint
ENTRYPOINT ["/self/env/entrypoint.sh"]
