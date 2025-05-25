# Define services incrementally
SERVICES += base
base_BUILD_DEPS :=
base_RUN_DEPS := proxy

SERVICES += android_emulator_ots
android_emulator_ots_BUILD_DEPS :=
android_emulator_ots_RUN_DEPS :=

# This takes forever to build...so dont use it.
SERVICES += android_emulator
android_emulator_BUILD_DEPS := android_sdk
android_emulator_RUN_DEPS :=

SERVICES += android_sdk
android_sdk_BUILD_DEPS := base
android_sdk_RUN_DEPS :=

SERVICES += android_studio
android_studio_BUILD_DEPS :=
android_studio_RUN_DEPS :=

SERVICES += flutter
flutter_BUILD_DEPS :=
flutter_RUN_DEPS := 

SERVICES += vscode
vscode_BUILD_DEPS := base
vscode_RUN_DEPS := andoird_sdk android_studio flutter android_emulator_ots

include ../docker/docker.mk

