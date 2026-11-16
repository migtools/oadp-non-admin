FROM registry.redhat.io/openshift/golang-builder:golang-builder-v1.26-rhel9 AS builder
COPY . .
WORKDIR $APP_ROOT/app/
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download
COPY cmd/main.go cmd/main.go
COPY api/ api/
COPY internal/ internal/
ENV BUILDTAGS strictfipsruntime
ENV GOEXPERIMENT strictfipsruntime
RUN CGO_ENABLED=1 GOOS=linux go build -tags "$BUILDTAGS" -mod=mod -a -o manager cmd/main.go

FROM registry.redhat.io/ubi9/ubi:latest
COPY --from=builder $APP_ROOT/app/manager /manager

USER 65532:65532

ENTRYPOINT ["/manager"]

LABEL description="OpenShift API for Data Protection - Non-Admin"
LABEL io.k8s.description="OpenShift API for Data Protection - Non-Admin"
LABEL io.k8s.display-name="OADP Non-Admin"
LABEL io.openshift.tags="migration"
LABEL summary="OpenShift API for Data Protection - Non-Admin"
