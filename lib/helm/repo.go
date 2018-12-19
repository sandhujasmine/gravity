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

package helm

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"

	"github.com/docker/docker/pkg/archive"
	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/pack"
	"github.com/gravitational/gravity/lib/utils"

	"k8s.io/helm/pkg/chartutil"
	"k8s.io/helm/pkg/provenance"
	"k8s.io/helm/pkg/repo"

	"github.com/ghodss/yaml"
	"github.com/gravitational/trace"
)

// Repository defines a Helm repository interface.
type Repository interface {
	FetchChart(loc.Locator) (io.ReadCloser, error)
	GetIndexFile() (io.Reader, error)
	// PutChart(chartName, chartVersion string, data io.Reader) error
	// DeleteChart(chartName, chartVersion string) error
	AddToIndex(loc.Locator) error
	RemoveFromIndex(loc.Locator) error
}

type Config struct {
	Packages pack.PackageService
}

type clusterRepository struct {
	Config
	sync.Mutex
}

func NewRepository(config Config) (*clusterRepository, error) {
	return &clusterRepository{
		Config: config,
	}, nil
}

func (r *clusterRepository) FetchChart(locator loc.Locator) (io.ReadCloser, error) {
	_, reader, err := r.Packages.ReadPackage(locator)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer reader.Close()
	tmpDir, err := ioutil.TempDir("", "package")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer os.RemoveAll(tmpDir)
	// unpack application resources into temporary directory
	err = archive.Untar(reader, tmpDir, &archive.TarOptions{
		NoLchown:        true,
		ExcludePatterns: []string{"registry"},
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	// load and package helm chart
	chart, err := chartutil.LoadDir(filepath.Join(tmpDir, "resources"))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	chartDir, err := ioutil.TempDir("", "chart")
	if err != nil {
		return nil, trace.Wrap(err)
	}
	path, err := chartutil.Save(chart, chartDir)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	chartReader, err := os.Open(path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &utils.CleanupReadCloser{
		ReadCloser: chartReader,
		Cleanup: func() {
			os.RemoveAll(chartDir)
		},
	}, nil
}

func (r *clusterRepository) GetIndexFile() (io.Reader, error) {
	indexFile, err := r.getIndexFile()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	data, err := yaml.Marshal(indexFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return bytes.NewReader(data), nil
}

func (r *clusterRepository) PutChart(name, version string, data io.Reader) error {
	locator, err := loc.NewLocator("charts", name, version)
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = r.Packages.CreatePackage(*locator, data)
	if err != nil {
		return trace.Wrap(err)
	}
	err = r.AddToIndex(*locator)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (r *clusterRepository) RemoveFromIndex(locator loc.Locator) error {
	r.Lock()
	defer r.Unlock()
	indexFile, err := r.getIndexFile()
	if err != nil {
		return trace.Wrap(err)
	}
	for name, versions := range indexFile.Entries {
		if name == locator.Name {
			for i, version := range versions {
				if version.Version == locator.Version {
					indexFile.Entries[name] = append(
						versions[:i], versions[i+1:]...)
					break
				}
			}
		}
	}
	data, err := yaml.Marshal(indexFile)
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = r.Packages.UpsertPackage(loc.ChartsLocator, bytes.NewReader(data))
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (r *clusterRepository) AddToIndex(locator loc.Locator) error {
	r.Lock()
	defer r.Unlock()
	_, reader, err := r.Packages.ReadPackage(locator)
	if err != nil {
		return trace.Wrap(err)
	}
	defer reader.Close()
	chart, err := chartutil.LoadArchive(reader)
	if err != nil {
		return trace.Wrap(err)
	}
	digest, err := r.digest(locator)
	if err != nil {
		return trace.Wrap(err)
	}
	indexFile, err := r.getIndexFile()
	if err != nil {
		return trace.Wrap(err)
	}
	if indexFile.Has(chart.Metadata.Name, chart.Metadata.Version) {
		return trace.AlreadyExists("chart %v:%v already exists",
			chart.Metadata.Name, chart.Metadata.Version)
	}
	url := fmt.Sprintf("%v/charts/%v-%v.tgz",
		r.Packages.PortalURL(), chart.Metadata.Name, chart.Metadata.Version)
	indexFile.Add(chart.Metadata, url, "", digest)
	indexFile.SortEntries()
	data, err := yaml.Marshal(indexFile)
	if err != nil {
		return trace.Wrap(err)
	}
	_, err = r.Packages.UpsertPackage(loc.ChartsLocator, bytes.NewReader(data))
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

func (r *clusterRepository) digest(locator loc.Locator) (string, error) {
	_, reader, err := r.Packages.ReadPackage(locator)
	if err != nil {
		return "", trace.Wrap(err)
	}
	defer reader.Close()
	digest, err := provenance.Digest(reader)
	if err != nil {
		return "", trace.Wrap(err)
	}
	return digest, nil
}

func (r *clusterRepository) getIndexFile() (*repo.IndexFile, error) {
	_, reader, err := r.Packages.ReadPackage(loc.ChartsLocator)
	if err != nil && !trace.IsNotFound(err) {
		return nil, trace.Wrap(err)
	}
	if trace.IsNotFound(err) {
		return repo.NewIndexFile(), nil
	}
	defer reader.Close()
	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	var indexFile repo.IndexFile
	err = yaml.Unmarshal(data, &indexFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &indexFile, nil
}

func (r *clusterRepository) DeleteChart(chartName, chartVersion string) error {
	locator, err := loc.NewLocator("charts", chartName, chartVersion)
	if err != nil {
		return trace.Wrap(err)
	}
	err = r.RemoveFromIndex(*locator)
	if err != nil {
		return trace.Wrap(err)
	}
	err = r.Packages.DeletePackage(*locator)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}
