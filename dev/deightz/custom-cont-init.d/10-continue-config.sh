#!/bin/bash
# Seed ~/.continue with repo defaults, which point Continue at the local
# models served by the og gateway (og service, http://og:11434).
mkdir -p /config/.continue
cp -rn /defaults/continue/. /config/.continue/
chown -R "$(stat -c '%u:%g' /config)" /config/.continue
