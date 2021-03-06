package cluster

import (
	"errors"
	"fmt"
	"github.com/Sirupsen/logrus"
	"sync"
)

var (
	ErrEngineNotConnected = errors.New("engine is not connected to docker's REST API")
	logger                = logrus.New()
)

type Cluster struct {
	mux             sync.Mutex
	engines         map[string]*Engine
	resourceManager *ResourceManager
}

func New(manager *ResourceManager, engines ...*Engine) (*Cluster, error) {
	c := &Cluster{
		engines:         make(map[string]*Engine),
		resourceManager: manager,
	}

	for _, e := range engines {
		if !e.IsConnected() {
			return nil, ErrEngineNotConnected
		}

		c.engines[e.ID] = e
	}

	return c, nil
}

func (c *Cluster) AddEngine(e *Engine) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	c.engines[e.ID] = e

	return nil
}

func (c *Cluster) RemoveEngine(e *Engine) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	delete(c.engines, e.ID)

	return nil
}

// ListContainers returns all the containers running in the cluster
func (c *Cluster) ListContainers(all bool, size bool, filter string) []*Container {
	out := []*Container{}

	for _, e := range c.engines {
		containers, _ := e.ListContainers(all, size, filter)

		out = append(out, containers...)
	}

	return out
}

func (c *Cluster) Kill(container *Container, sig int) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	engine := c.engines[container.Engine.ID]
	if engine == nil {
		return fmt.Errorf("engine with id %s is not in cluster", container.Engine.ID)
	}

	return engine.Kill(container, sig)
}

func (c *Cluster) Stop(container *Container) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	engine := c.engines[container.Engine.ID]
	if engine == nil {
		return fmt.Errorf("engine with id %s is not in cluster", container.Engine.ID)
	}

	return engine.Stop(container)
}

func (c *Cluster) Restart(container *Container, timeout int) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	engine := c.engines[container.Engine.ID]
	if engine == nil {
		return fmt.Errorf("engine with id %s is not in cluster", container.Engine.ID)
	}

	return engine.Restart(container, timeout)
}

func (c *Cluster) Remove(container *Container) error {
	c.mux.Lock()
	defer c.mux.Unlock()

	engine := c.engines[container.Engine.ID]
	if engine == nil {
		return fmt.Errorf("engine with id %s is not in cluster", container.Engine.ID)
	}

	return engine.Remove(container)
}

func (c *Cluster) Start(image *Image, pull bool) (*Container, error) {
	c.mux.Lock()
	defer c.mux.Unlock()

	var engineResources = []*EngineSnapshot{}

	for _, e := range c.engines {
		logger.Infof("engine %s is connected: %b", e.ID, e.IsConnected())
		containers, err := e.ListContainers(false, false, "")
		if err != nil {
			return nil, err
		}
		var cpus, memory float64
		for _, con := range containers {
			cpus += con.Image.Cpus
			memory += con.Image.Memory
		}

		engineResources = append(engineResources, &EngineSnapshot{
			ID:             e.ID,
			ReservedCpus:   cpus,
			ReservedMemory: memory,
			Cpus:           e.Cpus,
			Memory:         e.Memory,
		})
	}

	if len(engineResources) == 0 {
		return nil, fmt.Errorf("no eligible engines to run image")
	}

	container := &Container{
		Image: image,
		Name:  image.ContainerName,
	}

	logger.Infof("container name: %s, image name: %s",
		container.Name, container.Image.Name)

	s, err := c.resourceManager.PlaceContainer(container, engineResources)
	if err != nil {
		return nil, err
	}

	engine := c.engines[s.ID]

	if err := engine.Start(container, pull); err != nil {
		return nil, err
	}

	return container, nil
}

// Engines returns the engines registered in the cluster
func (c *Cluster) Engines() []*Engine {
	out := []*Engine{}

	for _, e := range c.engines {
		out = append(out, e)
	}

	return out
}

// Info returns information about the cluster
func (c *Cluster) ClusterInfo() *ClusterInfo {
	containerCount := 0
	imageCount := 0
	engineCount := len(c.engines)
	totalCpu := 0.0
	totalMemory := 0.0
	reservedCpus := 0.0
	reservedMemory := 0.0
	for _, e := range c.engines {
		c, err := e.ListContainers(false, false, "")
		if err != nil {
			// skip engines that are not available
			continue
		}
		for _, cnt := range c {
			reservedCpus += cnt.Image.Cpus
			reservedMemory += cnt.Image.Memory
		}
		i, err := e.ListImages()
		if err != nil {
			// skip engines that are not available
			continue
		}
		containerCount += len(c)
		imageCount += len(i)
		totalCpu += e.Cpus
		totalMemory += e.Memory
	}

	return &ClusterInfo{
		Cpus:           totalCpu,
		Memory:         totalMemory,
		ContainerCount: containerCount,
		ImageCount:     imageCount,
		EngineCount:    engineCount,
		ReservedCpus:   reservedCpus,
		ReservedMemory: reservedMemory,
	}
}

// Close signals to the cluster that no other actions will be applied
func (c *Cluster) Close() error {
	return nil
}
