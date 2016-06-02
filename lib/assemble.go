package sous

// Deployments returns all deployments described by the state.
func (s *State) Deployments() (Deployments, error) {
	ds := Deployments{}
	for _, m := range s.Manifests {
		deployments, err := s.DeploymentsFromManifest(m)
		if err != nil {
			return nil, err
		}
		ds = append(ds, deployments...)
	}
	return ds, nil
}

// DeploymentsFromManifest returns all deployments described by a single
// manifest, in terms of the wider state (i.e. global and cluster definitions
// and configuration).
func (s *State) DeploymentsFromManifest(m *Manifest) ([]*Deployment, error) {
	ds := []*Deployment{}
	inherit := DeploymentSpecs{}
	if global, ok := m.Deployments["Global"]; ok {
		inherit = append(inherit, global)
	}
	for clusterName, spec := range m.Deployments {
		spec.clusterName = clusterName
		d, err := BuildDeployment(m, spec, inherit)
		if err != nil {
			return nil, err
		}
		ds = append(ds, d)
	}
	return ds, nil
}