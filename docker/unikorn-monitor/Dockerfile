FROM gcr.io/distroless/static:nonroot

LABEL org.opencontainers.image.source=https://github.com/nscaledev/uni-kubernetes

# This is implcitly created by 'docker buildx build'
ARG TARGETARCH

COPY bin/${TARGETARCH}-linux-gnu/unikorn-monitor /

ENTRYPOINT ["/unikorn-monitor"]
