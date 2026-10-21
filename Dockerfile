FROM gotenberg/gotenberg:8.37.0-chromium@sha256:0d28ae9a96441588ef739623726bd500ad0720b77266c6f1351a13e333fbd61c AS build

# base image already has non-root "gotenberg" user (uid/gid 1001)
