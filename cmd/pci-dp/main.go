package main

import (
	"flag"
	"syscall"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
	resource "github.com/jonkeyguan/pci-device-plugin/pkg/pci-device"
	api "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

func main() {
	flag.Parse()
	defer glog.Flush()

	glog.Infof("Starting new Instance manager")
	var manager = resource.NewInstanceManager()

	glog.Infof("Starting new FS watcher")
	watcher, err := newFSWatcher(api.DevicePluginPath)
	if err != nil {
		glog.Fatal(err)
	}
	defer watcher.Close()

	glog.Infof("Starting new OS watcher")
	sigs := newOSWatcher(syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	restart := true

L:
	for {
		if restart {
			manager.StopInstances()

			var err error
			for _, instance := range manager.Instances {
				err = manager.StartInstance(instance)
				if err != nil {
					glog.Infof("Failed to start instance %v retrying", instance.ResourceName())
					break
				}
			}

			if err != nil {
				continue
			}

			restart = false
		}

		select {
		case event := <-watcher.Events:
			if (event.Name == api.KubeletSocket) && (event.Op&fsnotify.Create) == fsnotify.Create {
				glog.Infof("inotify: %v created, restarting", api.KubeletSocket)
				restart = true
			}
		case err := <-watcher.Errors:
			glog.Errorf("inotify: %v", err)
		case s := <-sigs:
			switch s {
			case syscall.SIGHUP:
				glog.Infof("Received SIGHUP, restarting.")
				restart = true
			default:
				glog.Infof("Received signal '%v', shutting down", s)
				manager.StopInstances()
				glog.Infof("Received signal '%v', shutting down", s)
				break L
			}
		}
	}
}
