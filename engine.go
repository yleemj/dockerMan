package dockerMan

import (
    "crypto/tls"
    "fmt"
    "net"
    "net/http"
    "time"
    "github.com/yleemj/dockerMan/app/cluster"
)

const (
    httpTimeout = time.Duration(1 * time.Second)
)

type (

    Engine struct {
        ID             string          `json:"id,omitempty" gorethink:"id,omitempty"`
        Engine         *cluster.Engine  `json:"engine,omitempty" gorethink:"engine,omitempty"`
        DockerVersion  string          `json:"docker_version,omitempty"`
    }
)

func dialTimeout(network, addr string) (net.Conn, error) {
    return net.DialTimeout(network, addr, httpTimeout)
}


func (e *Engine) Ping() (int, error) {
    status := 0
    addr := e.Engine.Addr
    tlsConfig := &tls.Config{}

    transport := http.Transport{
        Dial:            dialTimeout,
        TLSClientConfig: tlsConfig,
    }

    client := http.Client{
        Transport: &transport,
    }

    uri := fmt.Sprintf("%s/_ping", addr)
    resp, err := client.Get(uri)
    if err != nil {
        return 0, err
    } else {
        defer resp.Body.Close()
        status = resp.StatusCode
    }
    return status, nil
}
