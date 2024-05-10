# pci-device-plugin

switch arch type from M1  
```
arch -x86_64 /bin/zsh
```

update offline vendor  
```
go mod tidy
go mod vendor
go mod verify
```

image
```
quay.io/jonkey/pic-device-plugin
```
