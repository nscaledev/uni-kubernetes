FROM gcr.io/distroless/static:nonroot

LABEL org.opencontainers.image.source=https://github.com/nscaledev/uni-kubernetes

# This is implcitly created by 'docker buildx build'
ARG TARGETARCH

# Required as we are talking to Openstack public endpoints.
COPY bin/${TARGETARCH}-linux-gnu/unikorn-server /

ENTRYPOINT ["/unikorn-server"]
