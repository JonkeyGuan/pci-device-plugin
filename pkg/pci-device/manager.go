package resource

import (
	"strings"

	api "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type InstanceManager struct {
	Instances []*PCIDevicePluginInstance
}

type PCIDevicePluginInstance struct {
	devicePlugin *PCIDevicePlugin
	resourceName string
	iommuGroup   string
	pcieBDF      string
	socketName   string
}

func (m *PCIDevicePluginInstance) ResourceName() string {
	return m.resourceName
}

func NewInstanceManager() *InstanceManager {
	var instances []*PCIDevicePluginInstance
	devices := ScanDevices()

	for _, dev := range devices {
		var instance PCIDevicePluginInstance
		instance.devicePlugin = nil
		instance.iommuGroup = dev.iommuGroup
		instance.pcieBDF = dev.bdf
		instance.resourceName = "pci/dev-" + dev.name
		instance.socketName = api.DevicePluginPath + strings.ReplaceAll(instance.resourceName, "/", "-") + ".sock"
		instances = append(instances, &instance)
	}

	return &InstanceManager{
		Instances: instances,
	}
}

func (m *InstanceManager) StartInstance(instance *PCIDevicePluginInstance) error {
	instance.devicePlugin = NewDevicePlugin(instance)
	return instance.devicePlugin.Serve()
}

func (m *InstanceManager) StopInstances() error {
	for _, instance := range m.Instances {
		if instance.devicePlugin != nil {
			instance.devicePlugin.Stop()
		}
	}
	return nil
}
