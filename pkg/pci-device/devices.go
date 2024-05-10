package resource

import (
	"os"
	"regexp"
	"strings"

	"github.com/golang/glog"
)

type pciDevice struct {
	bdf        string
	deviceId   string
	vendorId   string
	iommuGroup string
	name       string
}

func ScanDevices() []pciDevice {
	var bdfs []string
	var devices []pciDevice

	path := "/sys/bus/pci/drivers/vfio-pci"
	bdfRegExp := regexp.MustCompile(`^[a-f0-9]{4}:[a-f0-9]{2}:[a-f0-9]{2}.[0-9]$`)
	iommuGroupRegExp := regexp.MustCompile(`\/(\d+)$`)

	list, err := os.ReadDir(path)
	if err != nil {
		glog.Fatal(err)
	}

	for _, file := range list {
		mode := file.Type()
		name := file.Name()
		if (mode & os.ModeSymlink) == os.ModeSymlink {
			if bdfRegExp.MatchString(name) {
				bdfs = append(bdfs, name)
			}
		}
	}

	for _, bdf := range bdfs {
		fullpath := path + "/" + bdf

		content, err := os.ReadFile(fullpath + "/vendor")
		if err != nil {
			glog.Error(err)
			continue
		}
		vendor := strings.TrimSpace(string(content))
		vendor = vendor[2:]

		content, err = os.ReadFile(fullpath + "/device")
		if err != nil {
			glog.Error(err)
			continue
		}
		device := strings.TrimSpace(string(content))
		device = device[2:]

		iommuGroup, err := os.Readlink(fullpath + "/iommu_group")
		if err != nil {
			glog.Error(err)
			continue
		}

		match := iommuGroupRegExp.FindStringSubmatch(iommuGroup)
		if len(match) == 0 {
			glog.Errorf("Failed to get IOMMU group")
			continue
		}
		iommuGroup = match[1]

		if _, err := os.Stat("/dev/vfio/" + iommuGroup); os.IsNotExist(err) {
			glog.Error(err)
			continue
		}

		glog.Infof("Found PCI device %v", bdf)
		glog.Infof("Vendor %v", vendor)
		glog.Infof("Device %v", device)
		glog.Infof("IOMMU Group %v", iommuGroup)

		devices = append(devices, pciDevice{
			bdf:        bdf,
			vendorId:   vendor,
			deviceId:   device,
			iommuGroup: iommuGroup,
			name:       strings.ReplaceAll(strings.ReplaceAll(bdf, ":", "-"), ".", "-"),
		})
	}

	return devices
}
