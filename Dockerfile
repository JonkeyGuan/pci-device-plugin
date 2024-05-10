FROM alpine
LABEL maintainer="jonkey.guan@gmail.com"

COPY pci-device-plugin /usr/bin/pci-device-plugin

ENTRYPOINT ["/usr/bin/pci-device-plugin", "-logtostderr=true", "-stderrthreshold=INFO", "-v=5"]
