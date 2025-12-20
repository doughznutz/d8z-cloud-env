# Doughznutz Development Environment

## Description

This project contains the development environment for Doughznutz LLC. It utilizes Docker and open-source tools and can be run in either the cloud or locally. Hooks are provided for GitHub. Secrets, like API keys, are managed in cloud secrets or in .env files.

## Installation

docker pull  doughznutz/doughznutz:d8z-cloud-env

## Usage

docker run \
    -it --rm \
    --env-file <(env) \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v /home/$USER/projects:/home/$USER/projects \
    doughznutz/doughznutz:d8z-cloud-env 

## Contributing

Email me: doughznutz@gmail.com

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
