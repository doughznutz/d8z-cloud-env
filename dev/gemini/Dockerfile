FROM node:latest

# Install git
RUN apt-get update && apt-get install -y git

 # Set the working directory
WORKDIR /gemini-cli

# Clone the repo
RUN git clone https://github.com/google-gemini/gemini-cli.git . && \
# ... and patch the PR that allows for proxy redirect.
# https://github.com/google-gemini/gemini-cli/pull/2899
    git fetch origin pull/2899/head:pr-2899 && \
    git checkout pr-2899

# Install dependencies
RUN npm install

# Build the project
RUN npm run build

# Install globally so we can run in the project dir.
RUN npm install -g .

# Create a non-root user
ARG USERNAME
RUN groupadd $USERNAME && useradd -ms /bin/bash -g $USERNAME $USERNAME

# Change ownership of the working directory
# RUN chown -R $USERNAME:$USERNAME /gemini-cli

# Switch to the non-root user
# USER $USERNAME

# Run gemini.
# CMD ["npm", "start"]
CMD ["bash", "-c", "cd /home/$USER/projects/$PROJECT && gemini"]

