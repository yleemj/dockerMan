package manager

import (
    "crypto/tls"
	"strings"
	"sync"

	"github.com/Sirupsen/logrus"
	"gopkg.in/mgo.v2"
    "gopkg.in/mgo.v2/bson"
	"github.com/gorilla/sessions"
	"github.com/yleemj/dockerMan"
    "github.com/yleemj/dockerMan/app/cluster"
)

const (
	tblNameConfig      = "config"
	storeKey           = "dockerMan"
	// trackerHost        = "http://tracker.shipyard-project.com"
	EngineHealthUp     = "up"
	EngineHealthDown   = "down"
)

var (
	logger                    = logrus.New()
	store                     = sessions.NewCookieStore([]byte(storeKey))
)

type (
	Manager struct {
		address          string
		database         string
		collection         string
		session          *mgo.Session
        mgoDB            *mgo.Database
		clusterManager   *cluster.Cluster
		engines          []*dockerMan.Engine
		store            *sessions.CookieStore
		StoreKey         string
		version          string
		disableUsageInfo bool
	}
)

func NewManager(addr string, database string, version string, disableUsageInfo bool) (*Manager, error) {
	session, err := mgo.Dial(addr)
    if err != nil {
            panic(err)
    }
    defer session.Close()

    db := session.DB(database)

	logger.Info("checking database")
    logger.Info("database: %s", db)

	//r.DbCreate(database).Run(session)

	m := &Manager{
		address:          addr,
		database:         database,
		session:          session,
        mgoDB:            db,
		store:            store,
		StoreKey:         storeKey,
		version:          version,
		disableUsageInfo: disableUsageInfo,
	}
	m.init()
	return m, nil
}

func (m *Manager) ClusterManager() *cluster.Cluster {
    return m.clusterManager
}


func (m *Manager) Store() *sessions.CookieStore {
	return m.store
}


func (m *Manager) init() []*dockerMan.Engine {
	engines := []*dockerMan.Engine{}
    err := m.mgoDB.C(tblNameConfig).Find(bson.M{}).All(&engines)
	if err != nil {
		logger.Fatalf("error getting configuration: %s", err)
	}

    logger.Infof("engines: %s", engines)

	m.engines = engines

    var engs []*cluster.Engine
    for _, d := range engines {
        tlsConfig := &tls.Config{}
        if err := setEngineClient(d.Engine, tlsConfig); err != nil {
            logger.Errorf("error setting tls config for engine: %s", err)
        }
        engs = append(engs, d.Engine)
        logger.Infof("loaded engine id=%s addr=%s", d.Engine.ID, d.Engine.Addr)
    }

    clusterManager, err := cluster.New(engs...)
    if err != nil {
        logger.Fatal(err)
    }

    m.clusterManager = clusterManager

	return engines
}


func (m *Manager) Engines() []*dockerMan.Engine {
	return m.engines
}

func (m *Manager) Engine(id string) *dockerMan.Engine {
	for _, e := range m.engines {
		if e.ID == id {
			return e
		}
	}
	return nil
}

func (m *Manager) Container(id string) (*cluster.Container, error) {
	containers := m.clusterManager.ListContainers(true, false, "")
	for _, cnt := range containers {
		if strings.HasPrefix(cnt.ID, id) {
			return cnt, nil
		}
	}
	return nil, nil
}


func (m *Manager) Containers(all bool) []*cluster.Container {
	return m.clusterManager.ListContainers(all, false, "")
}

func (m *Manager) ContainersByImage(name string, all bool) ([]*cluster.Container, error) {
	allContainers := m.Containers(all)
	imageContainers := []*cluster.Container{}
	for _, c := range allContainers {
		if strings.Index(c.Image.Name, name) > -1 {
			imageContainers = append(imageContainers, c)
		}
	}
	return imageContainers, nil
}

func (m *Manager) IdenticalContainers(container *cluster.Container, all bool) ([]*cluster.Container, error) {
	containers := []*cluster.Container{}
	imageContainers, err := m.ContainersByImage(container.Image.Name, all)
	if err != nil {
		return nil, err
	}
	for _, c := range imageContainers {
		args := len(c.Image.Args)
		origArgs := len(container.Image.Args)
		if c.Image.Memory == container.Image.Memory && args == origArgs && c.Image.Type == container.Image.Type {
			containers = append(containers, c)
		}
	}
	return containers, nil
}

func (m *Manager) ClusterInfo() *cluster.ClusterInfo {
	info := m.clusterManager.ClusterInfo()
	return info
}

func (m *Manager) Destroy(container *cluster.Container) error {
	if err := m.clusterManager.Kill(container, 9); err != nil {
		return err
	}
	if err := m.clusterManager.Remove(container); err != nil {
		return err
	}
	return nil
}


func (m *Manager) Run(image *cluster.Image, count int, pull bool) ([]*cluster.Container, error) {
	launched := []*cluster.Container{}

	var wg sync.WaitGroup
	wg.Add(count)
	var runErr error
	for i := 0; i < count; i++ {
		go func(wg *sync.WaitGroup) {
			container, err := m.clusterManager.Start(image, pull)
			if err != nil {
				runErr = err
			}
			launched = append(launched, container)
			wg.Done()
		}(&wg)
	}
	wg.Wait()
	return launched, runErr
}
