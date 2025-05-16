# How do use this stuff.
# When you login to your cloud instance, you need to pull this repo.
# CMD:

# Once you've done that, you can build the d8z-cloud-container
# CMD: docker build . -t d8z-cloud-container

# Then launch that container, in which you will now have better editing.
# CMD: docker run -it --rm --env-file .env -v .:/work d8z-cloud-container bash

# Open another SSH terminal so you have on inside the container, and one outside the container.  Your source code will be in "/work" directory.

# You can edit this code in the docker container SSH, and rebuild it in the other SSH.  Then reload the container using the command:

# Inside the container, cut and paste works through the SSH terminal. Highlight to copy, and ctrl+v to paste.

# This file was written in emacs inside the container.