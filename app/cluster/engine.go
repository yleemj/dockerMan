package cluster

import (
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"

	"github.com/samalba/dockerclient"
)

type Engine struct {
	ID     string   `json:"id,omitempty"`
	Addr   string   `json:"addr,omitempty"`
	Cpus   float64  `json:"cpus,omitempty"`
	Memory float64  `json:"memory,omitempty"`
	Labels []string `json:"labels,omitempty"`

	client     *dockerclient.DockerClient
	clientAuth *dockerclient.AuthConfig
}

func (e *Engine) Connect(config *tls.Config) error {
	logger.Infof("connect to docker daemon: %s", e.Addr)
	c, err := dockerclient.NewDockerClient(e.Addr, config)
	if err != nil {
		return err
	}

	e.client = c

	return nil
}

func (e *Engine) SetClient(c *dockerclient.DockerClient) {
	e.client = c
}

func (e *Engine) SetClientAuth(username, password, email string) {
	e.clientAuth = &dockerclient.AuthConfig{
		Username: username,
		Password: password,
		Email:    email,
	}
}

// IsConnected returns true if the engine is connected to a remote docker API
func (e *Engine) IsConnected() bool {
	return e.client != nil
}

func (e *Engine) Pull(image string) error {
	if err := e.client.PullImage(image, e.clientAuth); err != nil {
		return err
	}
	return nil
}

func (e *Engine) Start(c *Container, pullImage bool) error {
	var (
		err    error
		env    = []string{}
		client = e.client
		i      = c.Image
	)
	c.Engine = e

	logger.Infof("client: %v", client)

	for k, v := range i.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	env = append(env,
		fmt.Sprintf("_dockerMan_type=%s", i.Type),
		fmt.Sprintf("_dockerMan_labels=%s", strings.Join(i.Labels, ",")),
	)

	vols := make(map[string]struct{})
	binds := []string{}
	for _, v := range i.Volumes {
		if strings.Index(v, ":") > -1 {
			cv := strings.Split(v, ":")
			binds = append(binds, v)
			v = cv[1]
		}
		vols[v] = struct{}{}
	}

	config := &dockerclient.ContainerConfig{
		Hostname:     i.Hostname,
		Domainname:   i.Domainname,
		Image:        i.Name,
		Cmd:          i.Args,
		Memory:       int64(i.Memory) * 1024 * 1024,
		Env:          env,
		CpuShares:    int64(i.Cpus * 100.0 / e.Cpus),
		Cpuset:       i.Cpuset,
		ExposedPorts: make(map[string]struct{}),
		Volumes:      vols,
	}

	links := []string{}
	for k, v := range i.Links {
		links = append(links, fmt.Sprintf("%s:%s", k, v))
	}

	hostConfig := &dockerclient.HostConfig{
		PublishAllPorts: i.Publish,
		PortBindings:    make(map[string][]dockerclient.PortBinding),
		Links:           links,
		Binds:           binds,
		RestartPolicy: dockerclient.RestartPolicy{
			Name:              i.RestartPolicy.Name,
			MaximumRetryCount: i.RestartPolicy.MaximumRetryCount,
		},
		NetworkMode: i.NetworkMode,
		Privileged:  i.Privileged,
	}

	for _, b := range i.BindPorts {
		key := fmt.Sprintf("%d/%s", b.ContainerPort, b.Proto)
		config.ExposedPorts[key] = struct{}{}

		hostConfig.PortBindings[key] = []dockerclient.PortBinding{
			{
				HostIp:   b.HostIp,
				HostPort: fmt.Sprint(b.Port),
			},
		}
	}

	if pullImage {
		if err := e.Pull(i.Name); err != nil {
			return err
		}
	}

	logger.Infof("config hostname : %v", config.Hostname)
	logger.Infof("config image: %v", config.Image)
	logger.Infof("config command: %v", config.Cmd)
	logger.Infof("config memory: %v", config.Memory)
	logger.Infof("config cpu shares: %v", config.CpuShares)
	logger.Infof("config cpu set: %v", config.Cpuset)
	logger.Infof("config volumes: %v", config.Volumes)

	if c.ID, err = client.CreateContainer(config, c.Name); err != nil {
		return err
	}

	logger.Infof("container %s name %s created", c.ID, c.Name)
	logger.Infof("host config: %v", hostConfig)

	if err := client.StartContainer(c.ID, hostConfig); err != nil {
		return err
	}

	logger.Infof("container %s started", c.ID)

	return e.updatePortInformation(c)
}

func (e *Engine) ListImages() ([]string, error) {
	images, err := e.client.ListImages()
	if err != nil {
		return nil, err
	}

	out := []string{}

	for _, i := range images {
		for _, t := range i.RepoTags {
			out = append(out, t)
		}
	}

	return out, nil
}

func (e *Engine) updatePortInformation(c *Container) error {
	info, err := e.client.InspectContainer(c.ID)
	if err != nil {
		return err
	}

	return parsePortInformation(info, c)
}

func (e *Engine) ListContainers(all bool, size bool, filter string) ([]*Container, error) {
	out := []*Container{}

	c, err := e.client.ListContainers(all, size, filter)
	if err != nil {
		return nil, err
	}

	for _, ci := range c {
		cc, err := FromDockerContainer(ci.Id, ci.Image, e)
		if err != nil {
			return nil, err
		}

		out = append(out, cc)
	}

	return out, nil
}

func (e *Engine) Kill(container *Container, sig int) error {
	return e.client.KillContainer(container.ID, strconv.Itoa(sig))
}

func (e *Engine) Stop(container *Container) error {
	return e.client.StopContainer(container.ID, 8)
}

func (e *Engine) Restart(container *Container, timeout int) error {
	return e.client.RestartContainer(container.ID, timeout)
}

func (e *Engine) Remove(container *Container) error {
	return e.client.RemoveContainer(container.ID, true, true)
}

func (e *Engine) Version() (*dockerclient.Version, error) {
	return e.client.Version()
}

func (e *Engine) String() string {
	return fmt.Sprintf("engine %s addr %s", e.ID, e.Addr)
}
