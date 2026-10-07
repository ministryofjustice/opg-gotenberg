FROM gotenberg/gotenberg:8.37.0@sha256:f29984bd1e226bf1b93ba90af06000afa8b315853e99d27b9aaa41b93f15c769 AS build

# base image already has non-root "gotenberg" user (uid/gid 1001)
