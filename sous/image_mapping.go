package sous

import (
	"fmt"

	"github.com/opentable/sous/util/docker_registry"
)

type (
	// NameCache is a primative database for looking up SourceVersions based on Docker image names and vice versa.
	NameCache struct {
		registryClient docker_registry.Client
		DockerNameLookup
		SourceNameLookup
	}

	ImageName string

	SourceNameLookup map[ImageName]*SourceRecord
	DockerNameLookup map[SourceVersion]*SourceRecord

	NotModifiedErr struct{}

	NoImageNameFound struct {
		SourceVersion
	}

	NoSourceVersionFound struct {
		ImageName
	}

	SourceRecord struct {
		md docker_registry.Metadata
	}
)

func (e NoImageNameFound) Error() string {
	return fmt.Sprintf("No image name for %v", e.SourceVersion)
}

func (e NoSourceVersionFound) Error() string {
	return fmt.Sprintf("No image name for %v", e.ImageName)
}

func (e NotModifiedErr) Error() string {
	return "Not modified"
}

var theNameCache = NameCache{
	docker_registry.NewClient(),
	make(DockerNameLookup),
	make(SourceNameLookup),
}

func (sr *SourceRecord) SourceVersion() (SourceVersion, error) {
	return SourceVersionFromLabels(sr.md.Labels)
}

func (sr *SourceRecord) Update(other *SourceRecord) {
	sr.md = other.md
}

// GetSourceVersion retreives a source version for an image name, updating it from the server if necessary
// Each call to GetSourceVersion implies an HTTP request, although it may be abbreviated by the use of an etag.
func GetSourceVersion(in string) (SourceVersion, error) {
	return theNameCache.GetSourceVersion(in)
}

// InsertContainerRecord stores a SourceVersion/image name pair into the global name cache
func InsertContainerRecord(sv SourceVersion, in, etag string) error {
	return theNameCache.Insert(sv, in, etag)
}

func (nc *NameCache) Insert(sv SourceVersion, in, etag string) error {
	record := SourceRecord{docker_registry.Metadata{
		CanonicalName: in,
		AllNames:      []string{in},
		Etag:          etag,
		Labels:        sv.DockerLabels(),
	}}

	return nc.insertRecord(&record)
}

func (nc *NameCache) insertRecord(sr *SourceRecord) error {
	err := nc.insertSourceVersion(sr)
	if err != nil {
		return err
	}

	err = nc.insertDockerName(sr)
	if err != nil {
		return err
	}

	return nil
}

func (nc *NameCache) GetSourceVersion(in string) (SourceVersion, error) {
	sr, err := nc.getSourceRecord(ImageName(in))

	oldSV, err := sr.SourceVersion()

	md, err := nc.registryClient.GetImageMetadata(string(in), sr.md.Etag)
	if _, ok := err.(NotModifiedErr); ok {
		return oldSV, nil
	}
	if err != nil {
		return SourceVersion{}, err
	}
	newSR := SourceRecord{md}

	nsv, nerr := newSR.SourceVersion()
	osv, oerr := sr.SourceVersion()
	if newSR.md.CanonicalName == sr.md.CanonicalName || (nerr == nil && oerr == nil && nsv == osv) {
		newSR.md.AllNames = union(newSR.md.AllNames, sr.md.AllNames)
	}
	nc.insertRecord(&newSR)
	return newSR.SourceVersion()
}

func union(left, right []string) []string {
	set := make(map[string]struct{})
	for _, s := range left {
		set[s] = struct{}{}
	}

	for _, s := range right {
		set[s] = struct{}{}
	}

	res := make([]string, len(set))

	for k, _ := range set {
		res = append(res, k)
	}

	return res
}

func (sn SourceNameLookup) getSourceRecord(in ImageName) (*SourceRecord, error) {
	if sr, ok := sn[in]; ok {
		return sr, nil
	} else {
		return nil, NoSourceVersionFound{in}
	}
}

func (sn SourceNameLookup) GetSourceVersion(in string) (SourceVersion, error) {
	if sr, ok := sn[ImageName(in)]; ok {
		return sr.SourceVersion()
	} else {
		return SourceVersion{}, NoSourceVersionFound{ImageName(in)}
	}
}

func (sn SourceNameLookup) GetCanonicalName(in string) (string, error) {
	if sr, ok := sn[ImageName(in)]; ok {
		return sr.md.CanonicalName, nil
	} else {
		return "", NoSourceVersionFound{ImageName(in)}
	}
}

func (sn SourceNameLookup) insertSourceVersion(sr *SourceRecord) error {
	for _, n := range sr.md.AllNames {
		existing, yes := sn[ImageName(n)]
		if yes {
			existing.Update(sr)
		} else {
			sn[ImageName(n)] = sr
		}
	}
	return nil
}

func (dl DockerNameLookup) GetImageName(sv SourceVersion) (string, error) {
	if sr, ok := dl[sv]; ok {
		return sr.md.CanonicalName, nil
	} else {
		return "", NoImageNameFound{sv}
	}
}

func (dl DockerNameLookup) insertDockerName(sr *SourceRecord) error {
	sv, err := sr.SourceVersion()
	if err != nil {
		return err
	}

	existing, yes := dl[sv]
	if yes {
		existing.Update(sr)
	} else {
		dl[sv] = sr
	}
	return nil
}
