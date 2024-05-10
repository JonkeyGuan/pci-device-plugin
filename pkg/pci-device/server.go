package resource

import (
	"net"
	"os"
	"path"
	"strings"
	"time"
	"unicode"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	api "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

type PCIDevicePlugin struct {
	devs         []*api.Device
	socket       string
	resourceName string
	iommuGroup   string
	pcieBDF      string

	stop   chan interface{}
	server *grpc.Server
}

func (m *PCIDevicePlugin) cleanup() error {
	err := os.Remove(m.socket)
	if err == nil {
		glog.Infof("Removing file %s", m.socket)
		return nil
	}

	if os.IsNotExist(err) {
		return nil
	}

	return err
}

func dial(unixSocketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(
		unixSocketPath,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(timeout),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, err
	}

	return conn, nil
}

func NewDevicePlugin(ii *PCIDevicePluginInstance) *PCIDevicePlugin {
	var devices []*api.Device

	devices = append(devices, &api.Device{
		ID:     ii.pcieBDF,
		Health: api.Healthy,
	})

	return &PCIDevicePlugin{
		devs:         devices,
		socket:       ii.socketName,
		resourceName: ii.resourceName,
		iommuGroup:   ii.iommuGroup,
		pcieBDF:      ii.pcieBDF,
		stop:         make(chan interface{}),
	}
}

func (m *PCIDevicePlugin) Serve() error {
	err := m.Start()
	if err != nil {
		glog.Errorf("Could not start device plugin: %v", err)
		return err
	}
	glog.Infof("Starting to serve on %v", m.socket)

	err = m.Register(api.KubeletSocket, m.resourceName)
	if err != nil {
		glog.Errorf("Could not register device plugin: %v", err)
		m.Stop()
		return err
	}
	glog.Infof("Registered device plugin with Kubelet for %v", m.resourceName)

	return nil
}

func (m *PCIDevicePlugin) Register(kubeletEndpoint, resourceName string) error {
	conn, err := dial(kubeletEndpoint, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := api.NewRegistrationClient(conn)
	reqt := &api.RegisterRequest{
		Version:      api.Version,
		Endpoint:     path.Base(m.socket),
		ResourceName: resourceName,
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		return err
	}

	return nil
}

func (m *PCIDevicePlugin) Start() error {
	err := m.cleanup()
	if err != nil {
		return err
	}

	sock, err := net.Listen("unix", m.socket)
	if err != nil {
		return err
	}

	m.server = grpc.NewServer([]grpc.ServerOption{}...)
	api.RegisterDevicePluginServer(m.server, m)

	go m.server.Serve(sock)

	// Wait for server to start by launching a blocking connexion
	conn, err := dial(m.socket, 60*time.Second)
	if err != nil {
		return err
	}
	conn.Close()

	return nil
}

func (m *PCIDevicePlugin) Stop() error {
	glog.Infof("Stopping server with socket %v", m.socket)
	if m.server == nil {
		return nil
	}

	m.server.Stop()
	m.server = nil
	close(m.stop)
	glog.Infof("Server stopped with socket %v", m.socket)

	return m.cleanup()
}

func (m *PCIDevicePlugin) ListAndWatch(e *api.Empty, s api.DevicePlugin_ListAndWatchServer) error {
	s.Send(&api.ListAndWatchResponse{Devices: m.devs})
	for range m.stop {
		return nil
	}
	return nil
}

func (m *PCIDevicePlugin) Allocate(ctx context.Context, reqs *api.AllocateRequest) (*api.AllocateResponse, error) {
	responses := api.AllocateResponse{}

	for _, req := range reqs.ContainerRequests {
		var devices []*api.DeviceSpec

		for _, id := range req.DevicesIDs {
			glog.Infof("Allocated pci device %v", id)

			devices = append(devices, &api.DeviceSpec{
				ContainerPath: "/dev/vfio/vfio",
				HostPath:      "/dev/vfio/vfio",
				Permissions:   "mrw",
			})

			devices = append(devices, &api.DeviceSpec{
				ContainerPath: "/dev/vfio/" + m.iommuGroup,
				HostPath:      "/dev/vfio/" + m.iommuGroup,
				Permissions:   "mrw",
			})

			devices = append(devices, &api.DeviceSpec{
				ContainerPath: "/sys/bus/pci/devices/" + m.pcieBDF,
				HostPath:      "/sys/bus/pci/devices/" + m.pcieBDF,
				Permissions:   "mrw",
			})

			devices = append(devices, &api.DeviceSpec{
				ContainerPath: "/sys/bus/pci/drivers/vfio-pci/" + m.pcieBDF,
				HostPath:      "/sys/bus/pci/drivers/vfio-pci/" + m.pcieBDF,
				Permissions:   "mrw",
			})

			devices = append(devices, &api.DeviceSpec{
				ContainerPath: "/sys/kernel/iommu_groups/" + m.iommuGroup,
				HostPath:      "/sys/kernel/iommu_groups/" + m.iommuGroup,
				Permissions:   "mrw",
			})

			split := strings.Split(m.pcieBDF, ":")
			pp2 := strings.Join(split[:len(split)-1], ":")

			devices = append(devices, &api.DeviceSpec{
				ContainerPath: "/sys/devices/pci" + pp2,
				HostPath:      "/sys/devices/pci" + pp2,
				Permissions:   "mrw",
			})
		}

		envKey := formatEnvName("PCI_RESOURCE_" + m.resourceName)
		response := api.ContainerAllocateResponse{
			Devices: devices,
			Envs: map[string]string{
				envKey: m.pcieBDF,
			},
		}

		responses.ContainerResponses = append(responses.ContainerResponses, &response)
	}

	return &responses, nil
}

func (m *PCIDevicePlugin) PreStartContainer(context.Context, *api.PreStartContainerRequest) (*api.PreStartContainerResponse, error) {
	return &api.PreStartContainerResponse{}, nil
}

func (m *PCIDevicePlugin) GetPreferredAllocation(context.Context, *api.PreferredAllocationRequest) (*api.PreferredAllocationResponse, error) {
	return &api.PreferredAllocationResponse{}, nil
}

func (m *PCIDevicePlugin) GetDevicePluginOptions(context.Context, *api.Empty) (*api.DevicePluginOptions, error) {
	return &api.DevicePluginOptions{}, nil
}

func formatEnvName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= 'A' && r <= 'Z' ||
			r >= 'a' && r <= 'z' ||
			r >= '0' && r <= '9' ||

			// kubevirt does not replace - with _ when looking for the env D:
			r == '_' || r == '-' {

			b.WriteRune(unicode.ToUpper(r))
		} else {
			b.WriteString("_")
		}
	}
	return b.String()
}
