#
# FILE: ~/docker/tools/android_sdk/Dockerfile

# Download and install Android SDK Command-line Tools (replace with latest version if needed)
ARG COMMAND_LINE_TOOLS_VERSION=11076708_latest
ARG COMMAND_LINE_TOOLS_FILENAME=commandlinetools-linux-${COMMAND_LINE_TOOLS_VERSION}.zip
ARG COMMAND_LINE_TOOLS_URL=https://dl.google.com/android/repository/${COMMAND_LINE_TOOLS_FILENAME}

# Dont use a custom image as fetcher or you have to rebuild when it does.
FROM ubuntu:latest AS fetcher

# Install required dependencies, including X11 and related libraries
RUN apt-get update
RUN apt-get install -y wget

# Download and install Android SDK Command-line Tools (replace with latest version if needed)
ARG COMMAND_LINE_TOOLS_VERSION
ARG COMMAND_LINE_TOOLS_FILENAME
ARG COMMAND_LINE_TOOLS_URL
RUN wget --no-check-certificate ${COMMAND_LINE_TOOLS_URL} -O /tmp/${COMMAND_LINE_TOOLS_FILENAME}

# Start a new container and copy the files fetched above.
FROM tools:base
ARG COMMAND_LINE_TOOLS_VERSION
ARG COMMAND_LINE_TOOLS_FILENAME
COPY --from=fetcher /tmp/${COMMAND_LINE_TOOLS_FILENAME} /tmp/${COMMAND_LINE_TOOLS_FILENAME}

RUN apt-get update
RUN apt-get install -y openjdk-17-jdk lib32stdc++6 libc6-i386 \
    libxtst6 libxext6 libxrender1 libxi6 git wget curl zip \
    x11-apps xauth libx11-6 libx11-dev libncurses6

ENV ANDROID_SDK_ROOT=/opt/android-sdk
RUN unzip -q /tmp/${COMMAND_LINE_TOOLS_FILENAME} -d $ANDROID_SDK_ROOT
ENV PATH=$PATH:$ANDROID_SDK_ROOT/cmdline-tools/bin
RUN chmod -R a+rw $ANDROID_SDK_ROOT

# Install SDK components (accept licenses)
RUN yes | sdkmanager --sdk_root=$ANDROID_SDK_ROOT --licenses
RUN yes | sdkmanager --sdk_root=$ANDROID_SDK_ROOT "platform-tools"
RUN yes | sdkmanager --sdk_root=$ANDROID_SDK_ROOT "platforms;android-35"
RUN yes | sdkmanager --sdk_root=$ANDROID_SDK_ROOT "build-tools;35.0.0"

# Load the environment with: module load android_sdk
COPY . /root/env

# Entrypoint for the container
COPY *.conf /etc/supervisor/conf.d/
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]

