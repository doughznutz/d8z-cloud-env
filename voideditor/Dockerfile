# Use the official Node.js LTS image as the base
FROM node:22-bullseye AS builder

# Install required system dependencies
RUN apt-get update && apt-get install -y \
    git \
    libx11-dev \
    libxkbfile-dev \
    libsecret-1-dev \
    libgtk-3-dev \
    libnss3 \
    libxss1 \
    libasound2 \
    libxshmfence1 \
    libgbm-dev \
    && rm -rf /var/lib/apt/lists/*

# Set the working directory
WORKDIR /app

# Clone the VoidEditor repository
RUN git clone https://github.com/voideditor/void.git .

# Install Node.js dependencies
RUN npm install

# Build React
RUN NODE_OPTIONS="--max-old-space-size=8192" npm run buildreact

# Build the VoidEditor application
# RUN npm run watch
RUN npm run gulp vscode-linux-x64

# Copy in the hacks and rebuild
#COPY void.contribution.ts ./src/vs/workbench/contrib/void/browser/void.contribution.ts
#COPY voidApiKeyInjector.contribution.ts ./src/vs/workbench/contrib/void/browser/voidApiKeyInjector.contribution.ts
#COPY voidSettingsService.ts ./src/vs/workbench/contrib/void/common/voidSettingsService.ts
#COPY modelCapabilities.ts ./src/vs/workbench/contrib/void/common/modelCapabilities.ts
#RUN npm run gulp vscode-linux-x64

# Switch to a VNC based docker image.
#FROM dorowu/ubuntu-desktop-lxde-vnc
FROM env:dind
COPY --from=builder /VSCode-linux-x64 /usr/bin/voideditor
ENV PATH=/usr/bin/voideditor/bin:$PATH

# Tell voideditor to not worry about WSL.
ENV DONT_PROMPT_WSL_INSTALL=IS_THIS_NECESSARY

# Expose the port (if applicable; adjust as needed)
EXPOSE 6800
EXPOSE 80

# Clone the VoidEditor repository
ARG USERNAME
USER $USERNAME
RUN git clone https://github.com/voideditor/void.git /home/$USERNAME/projects/voideditor

# Set the default command to start VoidEditor
USER root
COPY *.conf /etc/supervisor/conf.d/
CMD ["supervisord", "-c", "/etc/supervisor/supervisord.conf"]