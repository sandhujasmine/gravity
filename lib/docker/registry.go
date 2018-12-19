/*
Copyright 2018 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package docker

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gravitational/gravity/lib/defaults"

	"github.com/docker/distribution/configuration"
	"github.com/docker/distribution/registry/handlers"
	"github.com/gravitational/logrus"
)

func init() {
	if _, err := os.Stat("/var/lib/gravity/secrets/kubelet.cert"); err != nil {
		return
	}
	logrus.Infof("== DEBUG == INJECTING CERTS INTO DEFAULT CLIENT")
	cert, err := tls.LoadX509KeyPair("/var/lib/gravity/secrets/kubelet.cert",
		"/var/lib/gravity/secrets/kubelet.key")
	if err != nil {
		panic(err)
	}
	caCert, err := ioutil.ReadFile("/var/lib/gravity/secrets/root.cert")
	if err != nil {
		panic(err)
	}
	caCertPool, err := x509.SystemCertPool()
	if err != nil {
		panic(err)
	}
	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}
	tlsConfig.BuildNameToCertificate()
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig:       tlsConfig,
	}
	http.DefaultTransport = transport
	http.DefaultClient = &http.Client{Transport: transport}
}

func NewRegistry(ctx context.Context) (http.Handler, error) {
	app := handlers.NewApp(ctx, &configuration.Configuration{
		Version: configuration.CurrentVersion,
		Storage: configuration.Storage{
			"cache": configuration.Parameters{
				"blobdescriptor": "inmemory",
			},
			"filesystem": configuration.Parameters{
				"rootdirectory": defaults.ClusterRegistryDir,
			},
		},
		Proxy: configuration.Proxy{
			RemoteURL: "https://leader.telekube.local:5000",
		},
		Auth: configuration.Auth{"gravity": nil},
	})
	// TODO What's this for?
	app.RegisterHealthChecks()
	return app, nil
}
