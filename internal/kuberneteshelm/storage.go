package kuberneteshelm

import (
	"encoding/json"
	"maps"

	chart "helm.sh/helm/v4/pkg/chart/v2"
	ri "helm.sh/helm/v4/pkg/release"
	release "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
)

// Rendering is frozen for execution, while Helm history retains the original
// Chart and values. Ordinary Helm tooling can still inspect the release.
type releaseDriver struct {
	driver.Driver
	plan     frozen
	original *chart.Chart
}

func cloneRelease(v ri.Releaser) (*release.Release, error) {
	r, e := releaseV1(v)
	if e != nil {
		return nil, e
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return nil, ErrInvalid
	}
	var copy release.Release
	if json.Unmarshal(raw, &copy) != nil {
		return nil, ErrInvalid
	}
	copy.Labels = maps.Clone(r.Labels)
	return &copy, nil
}
func (d *releaseDriver) decorateRead(value ri.Releaser) (ri.Releaser, error) {
	r, e := cloneRelease(value)
	if e != nil {
		return nil, e
	}
	target := d.plan.Request
	if target.Operation == "rollback" && r.Version == target.RollbackRevision || target.Operation == "uninstall" && r.Version == target.ExpectedRevision {
		r.Manifest = d.plan.Manifest
		for _, h := range r.Hooks {
			for _, frozen := range d.plan.Hooks {
				if h.Name == frozen.Name {
					h.Manifest = frozen.Manifest
				}
			}
		}
	}
	return r, nil
}
func (d *releaseDriver) Get(key string) (ri.Releaser, error) {
	v, e := d.Driver.Get(key)
	if e != nil {
		return nil, e
	}
	return d.decorateRead(v)
}
func (d *releaseDriver) Query(labels map[string]string) ([]ri.Releaser, error) {
	rows, e := d.Driver.Query(labels)
	if e != nil {
		return nil, e
	}
	for i, r := range rows {
		rows[i], e = d.decorateRead(r)
		if e != nil {
			return nil, e
		}
	}
	return rows, nil
}
func (d *releaseDriver) forWrite(value ri.Releaser) (ri.Releaser, error) {
	r, e := cloneRelease(value)
	if e != nil {
		return nil, e
	}
	target := d.plan.Request
	if r.Version == target.ExpectedRevision+1 || target.Operation == "uninstall" && r.Version == target.ExpectedRevision {
		if r.Labels == nil {
			r.Labels = map[string]string{}
		}
		r.Labels[markerAnnotation] = d.plan.Marker
		if d.original != nil {
			r.Chart = d.original
		}
	}
	return r, nil
}
func (d *releaseDriver) Create(key string, value ri.Releaser) error {
	v, e := d.forWrite(value)
	if e != nil {
		return e
	}
	return d.Driver.Create(key, v)
}
func (d *releaseDriver) Update(key string, value ri.Releaser) error {
	v, e := d.forWrite(value)
	if e != nil {
		return e
	}
	return d.Driver.Update(key, v)
}
