FROM public.ecr.aws/lambda/provided:al2 AS release-default

ARG BIN_NAME=consul-lambda-registrator
ARG VERSION
# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS TARGETARCH
# Export BIN_NAME for the CMD below, it can't see ARGs directly.
ENV BIN_NAME=$BIN_NAME
ENV VERSION=$VERSION
# This is the location of the releases.
ENV HASHICORP_RELEASES=https://releases.hashicorp.com

LABEL description="consul-lambda-registrator provides automatically registers Lambdas into Consul." \
      maintainer="Consul Team <consul@hashicorp.com>" \
      name=$BIN_NAME \
      release=$VERSION \
      summary="consul-lambda-registrator provides automatically registers Lambdas into Consul." \
      vendor="HashiCorp" \
      version=$VERSION \
      org.opencontainers.image.authors="Consul Team <consul@hashicorp.com>" \
      org.opencontainers.image.description="consul-lambda-registrator provides automatically registers Lambdas into Consul." \
      org.opencontainers.image.documentation="https://www.consul.io/docs/lambda" \
      org.opencontainers.image.source="https://github.com/hashicorp/consul-lambda-registrator" \
      org.opencontainers.image.title=$BIN_NAME \
      org.opencontainers.image.url="https://www.consul.io/" \
      org.opencontainers.image.vendor="HashiCorp" \
      org.opencontainers.image.version=$VERSION

ENTRYPOINT ["/bin/consul-lambda-registrator"]
COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /bin/
