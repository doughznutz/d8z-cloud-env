#
# FILE: ~/docker/tools/android-studio/Dockerfile

# Download Android Studio (replace with latest version if needed)
ARG ANDROID_STUDIO_VERSION=2024.2.2.13 # Updated version
ARG ANDROID_STUDIO_FILENAME=android-studio-${ANDROID_STUDIO_VERSION}-linux.tar.gz  # Updated filename
ARG ANDROID_STUDIO_URL=https://redirector.gvt1.com/edgedl/android/studio/ide-zips/${ANDROID_STUDIO_VERSION}/${ANDROID_STUDIO_FILENAME}

# Dont use a custom image as fetcher or you have to rebuild when it does.
FROM ubuntu:latest AS fetcher

# Install required dependencies, including X11 and related libraries
RUN apt-get update
RUN apt-get install -y wget

ARG ANDROID_STUDIO_VERSION
ARG ANDROID_STUDIO_FILENAME
ARG ANDROID_STUDIO_URL
RUN wget --no-check-certificate ${ANDROID_STUDIO_URL} -O /tmp/${ANDROID_STUDIO_FILENAME}

# Start a new container and copy the files fetched above.
FROM android:base
ARG ANDROID_STUDIO_VERSION
ARG ANDROID_STUDIO_FILENAME
ARG ANDROID_STUDIO_URL
COPY --from=fetcher /tmp/${ANDROID_STUDIO_FILENAME} /tmp/${ANDROID_STUDIO_FILENAME}

RUN apt-get update
RUN apt-get install -y openjdk-17-jdk lib32stdc++6 libc6-i386 \
    libxtst6 libxext6 libxrender1 libxi6 git wget curl zip \
    x11-apps xauth libx11-6 libx11-dev libncurses6

RUN mkdir /opt/android-studio
RUN tar -xf /tmp/${ANDROID_STUDIO_FILENAME} -C /opt
RUN chmod a+rw /opt/android-studio
ENV PATH=$PATH:/opt/android-studio/bin

# Install fonts (for better GUI rendering)
# RUN apt-get install -y fonts-ipafont-gothic fonts-wqy-zenhei fonts-tlwg-gothic fonts-kacst fonts-freefont-ttf

# Expose port for debugging (if needed)
# EXPOSE 8600

# Set DISPLAY environment variable (crucial for GUI)
ENV DISPLAY=:0

# Entrypoint to start Android Studio
# ENTRYPOINT ["/opt/android-studio/bin/studio.sh"]

# Load the environment with: module load android_studio
COPY . /root/env

# Entrypoint for the container
COPY *.conf /etc/supervisor/conf.d/
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]