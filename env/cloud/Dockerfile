FROM ubuntu:latest

# Update package lists and install dependencies
RUN apt-get update && apt-get install -y \
    python3 \
    python3-pip \
    git \
    curl \
    dos2unix \
    jq \
    vim \
    less \
    && rm -rf /var/lib/apt/lists/*

# Download and install the Google Cloud SDK
RUN curl https://dl.google.com/dl/cloudsdk/release/google-cloud-sdk.tar.gz > /tmp/google-cloud-sdk.tar.gz

RUN mkdir -p /usr/local/gcloud \
  && tar -C /usr/local/gcloud -xvf /tmp/google-cloud-sdk.tar.gz \
  && /usr/local/gcloud/google-cloud-sdk/install.sh

# Add gcloud to the PATH
ENV PATH $PATH:/usr/local/gcloud/google-cloud-sdk/bin

# Install google-api-python-client and google-auth-httplib2
#RUN pip3 install --no-cache-dir google-api-python-client google-auth-httplib2

# Set the working directory
WORKDIR /self

# Copy the current directory to /self
COPY . /self

RUN find /self -name *.sh -exec dos2unix {} +
RUN find /self -name *.sh -exec chmod +x {} +

# Set the entrypoint
ENTRYPOINT ["/self/env/cloud/entrypoint.sh"]
