FROM gotenberg/gotenberg:8.38.1-chromium@sha256:5da4ab7b8fe7326537369e17b661be765d9a2887f77722faacdbe6d55812fb96 AS build

# base image already has non-root "gotenberg" user (uid/gid 1001)
