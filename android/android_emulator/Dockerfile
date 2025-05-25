#
# FILE: ~/docker/tools/android_emulator/Dockerfile

# Download emulator package
ARG EMULATOR_TAG=google_apis
ARG EMULATOR_PACKAGE=x86_64-35_r09
ARG EMULATOR_FILENAME=${EMULATOR_PACKAGE}.zip
ARG EMULATOR_URL=https://dl.google.com/android/repository/sys-img/${EMULATOR_TAG}/${EMULATOR_FILENAME}

# Dont use a custom image as fetcher or you have to rebuild when it does.
FROM ubuntu:latest AS fetcher

# Install required dependencies, including X11 and related libraries
RUN apt-get update
RUN apt-get install -y wget

# Download emulator package
ARG EMULATOR_TAG
ARG EMULATOR_PACKAGE
ARG EMULATOR_FILENAME
ARG EMULATOR_URL
RUN wget --no-check-certificate ${EMULATOR_URL} -O /tmp/${EMULATOR_FILENAME}

# Start a new container and copy the files fetched above.
FROM android:sdk

ARG EMULATOR_TAG
ARG EMULATOR_PACKAGE
ARG EMULATOR_FILENAME
COPY --from=fetcher /tmp/${EMULATOR_FILENAME} /tmp/${EMULATOR_FILENAME}

RUN apt-get update
RUN apt-get install -y qemu-kvm zip

# Setup environment and path.
ENV ANDROID_HOME=/opt/android-sdk
ENV ANDROID_SDK_ROOT=/opt/android-sdk
ENV PATH=$PATH:$ANDROID_SDK_ROOT/cmdline-tools/bin:$ANDROID_SDK_ROOT/platform-tools

# Unzip and create the emulator AVD
# RUN yes | sdkmanager --sdk_root=$ANDROID_SDK_ROOT "emulator" "system-images;android-35;google_apis;x86_64"
RUN mkdir -p $ANDROID_SDK_ROOT/system-images/android-35/${EMULATOR_TAG}
RUN unzip /tmp/${EMULATOR_FILENAME} -d $ANDROID_SDK_ROOT/system-images/android-35/${EMULATOR_TAG}
#RUN export ANDROID_SDK_ROOT
#RUN echo "no" | avdmanager create avd -n flutterEmu -k "system-images;android-35;google_apis;x86_64" --device "pixel"
#RUN echo "no" | avdmanager create avd -n miniAVD -k "system-images;android-22;default;x86" --device "Nexus One"

# Setup KVM
ARG USERNAME
RUN groupadd -f kvm
RUN usermod -aG kvm $USERNAME
#RUN chmod a+wr /dev/kvm

# Load the environment with: module load android_sdk
COPY . /root/env

# Entrypoint for the container
COPY *.conf /etc/supervisor/conf.d/
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]

