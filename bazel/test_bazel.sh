#!/usr/bin/bash

echo $PWD

# Delete restriction and broken packages
rm .bazelversion
rm depsets/BUILD
rm generating_code/BUILD
rm generating_code/gen/BUILD

# Run bazel to see that it works.
bazel build ...
