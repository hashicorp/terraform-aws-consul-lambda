FROM public.ecr.aws/lambda/provided:al2 AS release-default

ARG BIN_NAME=consul-lambda-registrator
ARG PRODUCT_VERSION
# TARGETARCH and TARGETOS are set automatically when --platform is provided.
ARG TARGETOS TARGETARCH
# Export BIN_NAME for the CMD below, it can't see ARGs directly.
ENV BIN_NAME=$BIN_NAME
ENV VERSION=$PRODUCT_VERSION
# This is the location of the releases.
ENV HASHICORP_RELEASES=https://releases.hashicorp.com

LABEL description="consul-lambda-registrator automatically registers Lambda functions into Consul." \
      maintainer="Consul Team <consul@hashicorp.com>" \
      name=$BIN_NAME \
      release=$VERSION \
      summary="consul-lambda-registrator automatically registers Lambda functions into Consul." \
      vendor="HashiCorp" \
      version=$PRODUCT_VERSION \
      org.opencontainers.image.authors="Consul Team <consul@hashicorp.com>" \
      org.opencontainers.image.description="consul-lambda-registrator automatically registers Lambda functions into Consul." \
      org.opencontainers.image.documentation="https://www.consul.io/docs/lambda" \
      org.opencontainers.image.source="https://github.com/hashicorp/terraform-aws-consul-lambda" \
      org.opencontainers.image.title=$BIN_NAME \
      org.opencontainers.image.url="https://www.consul.io/" \
      org.opencontainers.image.vendor="HashiCorp" \
      org.opencontainers.image.version=$PRODUCT_VERSION

ENTRYPOINT ["/bin/consul-lambda-registrator"]
COPY dist/$TARGETOS/$TARGETARCH/$BIN_NAME /bin/
