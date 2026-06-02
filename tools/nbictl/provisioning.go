// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package nbictl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	provapipb "aalyria.com/spacetime/api/provisioning/v1alpha"
	provnbipb "aalyria.com/spacetime/tools/nbictl/provisioning"
)

// isUnimplementedError returns true if the error is a gRPC Unimplemented error.
func isUnimplementedError(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	return st.Code() == codes.Unimplemented
}

type ProvisioningResources struct {
	p2pSrTePolicies              map[string]*provapipb.P2PSrTePolicy
	p2pSrTePolicyCandidatePaths  map[string]*provapipb.P2PSrTePolicyCandidatePath
	p2mpSrTePolicies             map[string]*provapipb.P2MpSrTePolicy
	p2mpSrTePolicyCandidatePaths map[string]*provapipb.P2MpSrTePolicyCandidatePath
	downtimes                    map[string]*provapipb.Downtime
	protectionAssociationGroups  map[string]*provapipb.ProtectionAssociationGroup
	disjointAssociationGroups    map[string]*provapipb.DisjointAssociationGroup
	links                        map[string]*provapipb.Link
	geographicRegions            map[string]*provapipb.GeographicRegion
	emissionsLimits              map[string]*provapipb.EmissionsLimit
	pointingConstraints          map[string]*provapipb.PointingConstraint
}

func (pr *ProvisioningResources) String() string {
	keys := slices.Concat(
		lo.Keys(pr.p2pSrTePolicies),
		lo.Keys(pr.p2pSrTePolicyCandidatePaths),
		lo.Keys(pr.p2mpSrTePolicies),
		lo.Keys(pr.p2mpSrTePolicyCandidatePaths),
		lo.Keys(pr.downtimes),
		lo.Keys(pr.protectionAssociationGroups),
		lo.Keys(pr.disjointAssociationGroups),
		lo.Keys(pr.links),
		lo.Keys(pr.geographicRegions),
		lo.Keys(pr.emissionsLimits),
		lo.Keys(pr.pointingConstraints),
	)

	if len(keys) == 0 {
		return ""
	}

	slices.Sort(keys)

	return "- " + strings.Join(keys, "\n- ")
}

func marshalMap[T proto.Message](m map[string]T, marshaller protoFormat) map[string]string {
	return lo.MapValues(m, func(value T, _ string) string {
		return string(lo.Must(marshaller.marshal(value)))
	})
}

func (pr *ProvisioningResources) MarshalledString(marshaller protoFormat) string {
	entries := slices.Concat(
		lo.Entries(marshalMap(pr.p2pSrTePolicies, marshaller)),
		lo.Entries(marshalMap(pr.p2pSrTePolicyCandidatePaths, marshaller)),
		lo.Entries(marshalMap(pr.p2mpSrTePolicies, marshaller)),
		lo.Entries(marshalMap(pr.p2mpSrTePolicyCandidatePaths, marshaller)),
		lo.Entries(marshalMap(pr.downtimes, marshaller)),
		lo.Entries(marshalMap(pr.protectionAssociationGroups, marshaller)),
		lo.Entries(marshalMap(pr.disjointAssociationGroups, marshaller)),
		lo.Entries(marshalMap(pr.links, marshaller)),
		lo.Entries(marshalMap(pr.geographicRegions, marshaller)),
		lo.Entries(marshalMap(pr.emissionsLimits, marshaller)),
		lo.Entries(marshalMap(pr.pointingConstraints, marshaller)),
	)
	slices.SortFunc(entries, func(e1, e2 lo.Entry[string, string]) int { return strings.Compare(e1.Key, e2.Key) })

	sortedValues := lo.Map(entries, func(item lo.Entry[string, string], index int) string {
		return item.Value
	})
	return strings.Join(sortedValues, "\n")
}

func NewProvisioningResources() *ProvisioningResources {
	return &ProvisioningResources{
		p2pSrTePolicies:              map[string]*provapipb.P2PSrTePolicy{},
		p2pSrTePolicyCandidatePaths:  map[string]*provapipb.P2PSrTePolicyCandidatePath{},
		p2mpSrTePolicies:             map[string]*provapipb.P2MpSrTePolicy{},
		p2mpSrTePolicyCandidatePaths: map[string]*provapipb.P2MpSrTePolicyCandidatePath{},
		downtimes:                    map[string]*provapipb.Downtime{},
		protectionAssociationGroups:  map[string]*provapipb.ProtectionAssociationGroup{},
		disjointAssociationGroups:    map[string]*provapipb.DisjointAssociationGroup{},
		links:                        map[string]*provapipb.Link{},
		geographicRegions:            map[string]*provapipb.GeographicRegion{},
		emissionsLimits:              map[string]*provapipb.EmissionsLimit{},
		pointingConstraints:          map[string]*provapipb.PointingConstraint{},
	}
}

func (pr *ProvisioningResources) ResourceCount() int {
	return len(pr.p2pSrTePolicies) +
		len(pr.p2pSrTePolicyCandidatePaths) +
		len(pr.p2mpSrTePolicies) +
		len(pr.p2mpSrTePolicyCandidatePaths) +
		len(pr.downtimes) +
		len(pr.protectionAssociationGroups) +
		len(pr.disjointAssociationGroups) +
		len(pr.links) +
		len(pr.geographicRegions) +
		len(pr.emissionsLimits) +
		len(pr.pointingConstraints)
}

func (pr *ProvisioningResources) InsertProvisioningResources(resources *provnbipb.ProvisioningResources) {
	pr.insertP2PSrTePolicies(resources.GetP2PSrTePolicies())
	pr.insertP2PSrTePolicyCandidatePaths(resources.GetP2PSrTePolicyCandidatePaths())
	pr.insertP2MpSrTePolicies(resources.GetP2MpSrTePolicies())
	pr.insertP2MpSrTePolicyCandidatePaths(resources.GetP2MpSrTePolicyCandidatePaths())
	pr.insertDowntimes(resources.GetDowntimes())
	pr.insertProtectionAssociationGroups(resources.GetProtectionAssociationGroups())
	pr.insertDisjointAssociationGroups(resources.GetDisjointAssociationGroups())
	pr.insertLinks(resources.GetLinks())
	pr.insertGeographicRegions(resources.GetGeographicRegions())
	pr.insertEmissionsLimits(resources.GetEmissionsLimits())
	pr.insertPointingConstraints(resources.GetPointingConstraints())
}

func ProvisioningResourcesFromRemote(ctx context.Context, client provapipb.ProvisioningClient, listBar *progressBar) (*ProvisioningResources, error) {
	pr := NewProvisioningResources()

	var (
		totalMu       sync.Mutex
		expandedTotal = 9
	)
	bumpTotal := func(delta int) {
		totalMu.Lock()
		defer totalMu.Unlock()
		expandedTotal += delta
		listBar.SetTotal(expandedTotal)
	}

	p := pool.New().WithErrors()

	var downtimes []*provapipb.Downtime
	p.Go(func() error {
		result, err := client.ListDowntimes(ctx, &provapipb.ListDowntimesRequest{})
		if err != nil {
			if isUnimplementedError(err) {
				listBar.Incr()
				return nil
			}
			return err
		}
		downtimes = result.GetDowntimes()
		listBar.Incr()
		return nil
	})

	var protectionAssociationGroups []*provapipb.ProtectionAssociationGroup
	p.Go(func() error {
		result, err := client.ListProtectionAssociationGroups(ctx, &provapipb.ListProtectionAssociationGroupsRequest{})
		if err != nil {
			if isUnimplementedError(err) {
				listBar.Incr()
				return nil
			}
			return err
		}
		protectionAssociationGroups = result.GetProtectionAssociationGroups()
		listBar.Incr()
		return nil
	})

	var disjointAssociationGroups []*provapipb.DisjointAssociationGroup
	p.Go(func() error {
		result, err := client.ListDisjointAssociationGroups(ctx, &provapipb.ListDisjointAssociationGroupsRequest{})
		if err != nil {
			if isUnimplementedError(err) {
				listBar.Incr()
				return nil
			}
			return err
		}
		disjointAssociationGroups = result.GetDisjointAssociationGroups()
		listBar.Incr()
		return nil
	})

	var links []*provapipb.Link
	p.Go(func() error {
		result, err := client.ListLinks(ctx, &provapipb.ListLinksRequest{})
		if err != nil {
			if isUnimplementedError(err) {
				listBar.Incr()
				return nil
			}
			return err
		}
		links = result.GetLinks()
		listBar.Incr()
		return nil
	})

	var geographicRegions []*provapipb.GeographicRegion
	p.Go(func() error {
		result, err := client.ListGeographicRegions(ctx, &provapipb.ListGeographicRegionsRequest{})
		if err != nil {
			if isUnimplementedError(err) {
				listBar.Incr()
				return nil
			}
			return err
		}
		geographicRegions = result.GetGeographicRegions()
		listBar.Incr()
		return nil
	})

	var emissionsLimits []*provapipb.EmissionsLimit
	p.Go(func() error {
		result, err := client.ListEmissionsLimits(ctx, &provapipb.ListEmissionsLimitsRequest{})
		if err != nil {
			if isUnimplementedError(err) {
				listBar.Incr()
				return nil
			}
			return err
		}
		emissionsLimits = result.GetEmissionsLimits()
		listBar.Incr()
		return nil
	})

	var pointingConstraints []*provapipb.PointingConstraint
	p.Go(func() error {
		result, err := client.ListPointingConstraints(ctx, &provapipb.ListPointingConstraintsRequest{})
		if err != nil {
			if isUnimplementedError(err) {
				listBar.Incr()
				return nil
			}
			return err
		}
		pointingConstraints = result.GetPointingConstraints()
		listBar.Incr()
		return nil
	})

	var (
		p2pSrTePolicies   []*provapipb.P2PSrTePolicy
		p2pCandidatePaths []*provapipb.P2PSrTePolicyCandidatePath
	)
	p.Go(func() error {
		result, err := client.ListP2PSrTePolicies(ctx, &provapipb.ListP2PSrTePoliciesRequest{})
		if err != nil {
			if isUnimplementedError(err) {
				listBar.Incr()
				return nil
			}
			return err
		}
		p2pSrTePolicies = result.GetP2PSrTePolicies()
		listBar.Incr()
		if len(p2pSrTePolicies) == 0 {
			return nil
		}
		sort.Slice(p2pSrTePolicies, func(i, j int) bool {
			return p2pSrTePolicies[i].GetName() < p2pSrTePolicies[j].GetName()
		})
		bumpTotal(len(p2pSrTePolicies))

		candidatePathResults := make([][]*provapipb.P2PSrTePolicyCandidatePath, len(p2pSrTePolicies))
		inner := pool.New().WithErrors()
		for i, policy := range p2pSrTePolicies {
			inner.Go(func() error {
				result, err := client.ListP2PSrTePolicyCandidatePaths(ctx, &provapipb.ListP2PSrTePolicyCandidatePathsRequest{
					Parent: policy.GetName(),
				})
				if err != nil {
					if isUnimplementedError(err) {
						listBar.Incr()
						return nil
					}
					return err
				}
				candidatePathResults[i] = result.GetP2PSrTePolicyCandidatePaths()
				listBar.Incr()
				return nil
			})
		}
		if err := inner.Wait(); err != nil {
			return err
		}
		p2pCandidatePaths = slices.Concat(candidatePathResults...)
		return nil
	})

	var (
		p2mpSrTePolicies   []*provapipb.P2MpSrTePolicy
		p2mpCandidatePaths []*provapipb.P2MpSrTePolicyCandidatePath
	)
	p.Go(func() error {
		result, err := client.ListP2MpSrTePolicies(ctx, &provapipb.ListP2MpSrTePoliciesRequest{})
		if err != nil {
			if isUnimplementedError(err) {
				listBar.Incr()
				return nil
			}
			return err
		}
		p2mpSrTePolicies = result.GetP2MpSrTePolicies()
		listBar.Incr()
		if len(p2mpSrTePolicies) == 0 {
			return nil
		}
		sort.Slice(p2mpSrTePolicies, func(i, j int) bool {
			return p2mpSrTePolicies[i].GetName() < p2mpSrTePolicies[j].GetName()
		})
		bumpTotal(len(p2mpSrTePolicies))

		candidatePathResults := make([][]*provapipb.P2MpSrTePolicyCandidatePath, len(p2mpSrTePolicies))
		inner := pool.New().WithErrors()
		for i, policy := range p2mpSrTePolicies {
			inner.Go(func() error {
				result, err := client.ListP2MpSrTePolicyCandidatePaths(ctx, &provapipb.ListP2MpSrTePolicyCandidatePathsRequest{
					Parent: policy.GetName(),
				})
				if err != nil {
					if isUnimplementedError(err) {
						listBar.Incr()
						return nil
					}
					return err
				}
				candidatePathResults[i] = result.GetP2MpSrTePolicyCandidatePaths()
				listBar.Incr()
				return nil
			})
		}
		if err := inner.Wait(); err != nil {
			return err
		}
		p2mpCandidatePaths = slices.Concat(candidatePathResults...)
		return nil
	})

	if err := p.Wait(); err != nil {
		return nil, err
	}

	pr.insertDowntimes(downtimes)
	pr.insertProtectionAssociationGroups(protectionAssociationGroups)
	pr.insertDisjointAssociationGroups(disjointAssociationGroups)
	pr.insertLinks(links)
	pr.insertGeographicRegions(geographicRegions)
	pr.insertEmissionsLimits(emissionsLimits)
	pr.insertPointingConstraints(pointingConstraints)
	pr.insertP2PSrTePolicies(p2pSrTePolicies)
	pr.insertP2PSrTePolicyCandidatePaths(p2pCandidatePaths)
	pr.insertP2MpSrTePolicies(p2mpSrTePolicies)
	pr.insertP2MpSrTePolicyCandidatePaths(p2mpCandidatePaths)

	return pr, nil
}

func (pr *ProvisioningResources) insertP2PSrTePolicies(entries []*provapipb.P2PSrTePolicy) {
	for _, entry := range entries {
		pr.p2pSrTePolicies[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertP2PSrTePolicyCandidatePaths(entries []*provapipb.P2PSrTePolicyCandidatePath) {
	for _, entry := range entries {
		pr.p2pSrTePolicyCandidatePaths[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertP2MpSrTePolicies(entries []*provapipb.P2MpSrTePolicy) {
	for _, entry := range entries {
		pr.p2mpSrTePolicies[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertP2MpSrTePolicyCandidatePaths(entries []*provapipb.P2MpSrTePolicyCandidatePath) {
	for _, entry := range entries {
		pr.p2mpSrTePolicyCandidatePaths[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertDowntimes(entries []*provapipb.Downtime) {
	for _, entry := range entries {
		pr.downtimes[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertProtectionAssociationGroups(entries []*provapipb.ProtectionAssociationGroup) {
	for _, entry := range entries {
		pr.protectionAssociationGroups[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertDisjointAssociationGroups(entries []*provapipb.DisjointAssociationGroup) {
	for _, entry := range entries {
		pr.disjointAssociationGroups[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertLinks(entries []*provapipb.Link) {
	for _, entry := range entries {
		pr.links[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertGeographicRegions(entries []*provapipb.GeographicRegion) {
	for _, entry := range entries {
		pr.geographicRegions[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertEmissionsLimits(entries []*provapipb.EmissionsLimit) {
	for _, entry := range entries {
		pr.emissionsLimits[entry.GetName()] = entry
	}
}

func (pr *ProvisioningResources) insertPointingConstraints(entries []*provapipb.PointingConstraint) {
	for _, entry := range entries {
		pr.pointingConstraints[entry.GetName()] = entry
	}
}

func provisioningResourcesAreEquivalent[T proto.Message](a, b T) bool {
	// TODO: find a more robust equivalency check.
	return proto.Equal(a, b)
}

func countValues(m map[string][]string) int {
	count := 0
	for _, valueSlice := range m {
		count += len(valueSlice)
	}

	return count
}

func ProvisioningSync(appCtx *cli.Context) error {
	ctx := appCtx.Context
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	if !appCtx.Args().Present() {
		return fmt.Errorf("sync needs at least one directory or filename argument")
	}

	// Step 1: load up all local provisioning resources.
	localResources := NewProvisioningResources()

	localFiles, err := findAllFilesWithExtension(marshaller.fileExt, appCtx.Bool("recursive"), appCtx.Args().Slice()...)
	if err != nil {
		return err
	}
	if len(localFiles) == 0 {
		return fmt.Errorf("found no local files to read")
	}

	showProgress := shouldShowProgress(appCtx)
	progress := newSyncProgress(showProgress)
	defer progress.Stop()

	readBar := progress.AddBar("reading files", len(localFiles))
	listBar := progress.AddBar("listing remote", 9)
	progress.Start()

	w := progress.Writer()

	for _, localFile := range localFiles {
		fmt.Fprintf(w, "reading %s\n", localFile)
		contents, err := os.ReadFile(localFile)
		if err != nil {
			return err
		}

		resources := &provnbipb.ProvisioningResources{}
		err = marshaller.unmarshal(contents, resources)
		if err != nil {
			return err
		}
		localResources.InsertProvisioningResources(resources)
		readBar.Incr()
	}
	if localResources.ResourceCount() == 0 {
		return fmt.Errorf("found no local resources to sync")
	}
	fmt.Fprintf(w, "\nfound local resources:\n%s", localResources)

	// Step 2: load up all the remote instance resources.
	maxConcurrency := appCtx.Int("max-concurrency")
	nextClient, closeAll, err := newProvisioningClientPool(appCtx, maxConcurrency)
	if err != nil {
		return err
	}
	defer closeAll()
	remoteResources, err := ProvisioningResourcesFromRemote(ctx, nextClient(), listBar)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\nfound remote resources:\n%s", remoteResources)

	// Step 3: compute and print/enact differences.
	deleteMode := appCtx.Bool("delete")
	dryRunMode := appCtx.Bool("dry-run")
	verboseMode := appCtx.Bool("verbose")
	printMode := dryRunMode || verboseMode

	// TODO: restructure these computations for scalability.
	resourcesToBeAdded := map[string][]string{
		"p2pSrTePolicies":             lo.Without(lo.Keys(localResources.p2pSrTePolicies), lo.Keys(remoteResources.p2pSrTePolicies)...),
		"p2pSrTePolicyCandidatePaths": lo.Without(lo.Keys(localResources.p2pSrTePolicyCandidatePaths), lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths)...),
		"downtimes":                   lo.Without(lo.Keys(localResources.downtimes), lo.Keys(remoteResources.downtimes)...),
		"protectionAssociationGroups": lo.Without(lo.Keys(localResources.protectionAssociationGroups), lo.Keys(remoteResources.protectionAssociationGroups)...),
		"disjointAssociationGroups":   lo.Without(lo.Keys(localResources.disjointAssociationGroups), lo.Keys(remoteResources.disjointAssociationGroups)...),
		"links":                       lo.Without(lo.Keys(localResources.links), lo.Keys(remoteResources.links)...),
		"geographicRegions":           lo.Without(lo.Keys(localResources.geographicRegions), lo.Keys(remoteResources.geographicRegions)...),
		"emissionsLimits":             lo.Without(lo.Keys(localResources.emissionsLimits), lo.Keys(remoteResources.emissionsLimits)...),
		"pointingConstraints":         lo.Without(lo.Keys(localResources.pointingConstraints), lo.Keys(remoteResources.pointingConstraints)...),
	}

	resourcesInCommon := map[string][]string{
		"p2pSrTePolicies":             lo.Intersect(lo.Keys(localResources.p2pSrTePolicies), lo.Keys(remoteResources.p2pSrTePolicies)),
		"p2pSrTePolicyCandidatePaths": lo.Intersect(lo.Keys(localResources.p2pSrTePolicyCandidatePaths), lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths)),
		"downtimes":                   lo.Intersect(lo.Keys(localResources.downtimes), lo.Keys(remoteResources.downtimes)),
		"protectionAssociationGroups": lo.Intersect(lo.Keys(localResources.protectionAssociationGroups), lo.Keys(remoteResources.protectionAssociationGroups)),
		"disjointAssociationGroups":   lo.Intersect(lo.Keys(localResources.disjointAssociationGroups), lo.Keys(remoteResources.disjointAssociationGroups)),
		"links":                       lo.Intersect(lo.Keys(localResources.links), lo.Keys(remoteResources.links)),
		"geographicRegions":           lo.Intersect(lo.Keys(localResources.geographicRegions), lo.Keys(remoteResources.geographicRegions)),
		"emissionsLimits":             lo.Intersect(lo.Keys(localResources.emissionsLimits), lo.Keys(remoteResources.emissionsLimits)),
		"pointingConstraints":         lo.Intersect(lo.Keys(localResources.pointingConstraints), lo.Keys(remoteResources.pointingConstraints)),
	}

	fmt.Fprintln(w, "\ncomparing local and remote resources:")
	fmt.Fprintf(w, "- %d resources to be added\n", countValues(resourcesToBeAdded))
	fmt.Fprintf(w, "- %d resources to be evaluated for update\n", countValues(resourcesInCommon))

	errs := []error{}

	///
	// Maybe delete resources.
	var deleteTotal int
	if deleteMode {
		deleteParams := deleteProvisioningParams{
			p2pSrTePolicies:             lo.Without(lo.Keys(remoteResources.p2pSrTePolicies), lo.Keys(localResources.p2pSrTePolicies)...),
			p2pSrTePolicyCandidatePaths: lo.Without(lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths), lo.Keys(localResources.p2pSrTePolicyCandidatePaths)...),
			downtimes:                   lo.Without(lo.Keys(remoteResources.downtimes), lo.Keys(localResources.downtimes)...),
			protectionAssociationGroups: lo.Without(lo.Keys(remoteResources.protectionAssociationGroups), lo.Keys(localResources.protectionAssociationGroups)...),
			disjointAssociationGroups:   lo.Without(lo.Keys(remoteResources.disjointAssociationGroups), lo.Keys(localResources.disjointAssociationGroups)...),
			links:                       lo.Without(lo.Keys(remoteResources.links), lo.Keys(localResources.links)...),
			geographicRegions:           lo.Without(lo.Keys(remoteResources.geographicRegions), lo.Keys(localResources.geographicRegions)...),
			emissionsLimits:             lo.Without(lo.Keys(remoteResources.emissionsLimits), lo.Keys(localResources.emissionsLimits)...),
			pointingConstraints:         lo.Without(lo.Keys(remoteResources.pointingConstraints), lo.Keys(localResources.pointingConstraints)...),

			w:              w,
			printMode:      printMode,
			dryRunMode:     dryRunMode,
			maxConcurrency: maxConcurrency,
			nextClient:     nextClient,
		}

		deleteTotal = len(slices.Concat(
			deleteParams.p2pSrTePolicies,
			deleteParams.p2pSrTePolicyCandidatePaths,
			deleteParams.downtimes,
			deleteParams.protectionAssociationGroups,
			deleteParams.disjointAssociationGroups,
			deleteParams.links,
			deleteParams.geographicRegions,
			deleteParams.emissionsLimits,
			deleteParams.pointingConstraints,
		))
		fmt.Fprintf(w, "- %d resources to be deleted\n", deleteTotal)

		deleteBar := progress.AddBar("deleting resources", deleteTotal)
		deleteParams.onProgress = deleteBar.Incr

		err := deleteProvisioning(ctx, deleteParams)
		errs = append(errs, err)
	}

	syncBar := progress.AddBar("syncing resources", countValues(resourcesToBeAdded)+countValues(resourcesInCommon))

	// Update and create resources (except P2P candidate paths which must wait for policies).
	p := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)

	updateRemoteResources(p, ctx, resourcesInCommon["p2pSrTePolicies"], localResources.p2pSrTePolicies, remoteResources.p2pSrTePolicies, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, policy *provapipb.P2PSrTePolicy) error {
		_, err := nextClient().UpdateP2PSrTePolicy(ctx, &provapipb.UpdateP2PSrTePolicyRequest{
			Policy:       policy,
			AllowMissing: false,
		})
		return err
	})
	updateRemoteResources(p, ctx, resourcesInCommon["p2pSrTePolicyCandidatePaths"], localResources.p2pSrTePolicyCandidatePaths, remoteResources.p2pSrTePolicyCandidatePaths, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, path *provapipb.P2PSrTePolicyCandidatePath) error {
		_, err := nextClient().UpdateP2PSrTePolicyCandidatePath(ctx, &provapipb.UpdateP2PSrTePolicyCandidatePathRequest{
			Path:         path,
			AllowMissing: false,
		})
		return err
	})
	updateRemoteResources(p, ctx, resourcesInCommon["downtimes"], localResources.downtimes, remoteResources.downtimes, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, downtime *provapipb.Downtime) error {
		_, err := nextClient().UpdateDowntime(ctx, &provapipb.UpdateDowntimeRequest{
			Downtime:     downtime,
			AllowMissing: false,
		})
		return err
	})
	updateRemoteResources(p, ctx, resourcesInCommon["protectionAssociationGroups"], localResources.protectionAssociationGroups, remoteResources.protectionAssociationGroups, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, protectionAssociationGroup *provapipb.ProtectionAssociationGroup) error {
		_, err := nextClient().UpdateProtectionAssociationGroup(ctx, &provapipb.UpdateProtectionAssociationGroupRequest{
			ProtectionAssociationGroup: protectionAssociationGroup,
			AllowMissing:               false,
		})
		return err
	})
	updateRemoteResources(p, ctx, resourcesInCommon["disjointAssociationGroups"], localResources.disjointAssociationGroups, remoteResources.disjointAssociationGroups, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, disjointAssociationGroup *provapipb.DisjointAssociationGroup) error {
		_, err := nextClient().UpdateDisjointAssociationGroup(ctx, &provapipb.UpdateDisjointAssociationGroupRequest{
			DisjointAssociationGroup: disjointAssociationGroup,
			AllowMissing:             false,
		})
		return err
	})
	updateRemoteResources(p, ctx, resourcesInCommon["links"], localResources.links, remoteResources.links, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, link *provapipb.Link) error {
		_, err := nextClient().UpdateLink(ctx, &provapipb.UpdateLinkRequest{
			Link:         link,
			AllowMissing: false,
		})
		return err
	})
	updateRemoteResources(p, ctx, resourcesInCommon["geographicRegions"], localResources.geographicRegions, remoteResources.geographicRegions, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, geographicRegion *provapipb.GeographicRegion) error {
		_, err := nextClient().UpdateGeographicRegion(ctx, &provapipb.UpdateGeographicRegionRequest{
			GeographicRegion: geographicRegion,
			AllowMissing:     false,
		})
		return err
	})
	updateRemoteResources(p, ctx, resourcesInCommon["emissionsLimits"], localResources.emissionsLimits, remoteResources.emissionsLimits, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, emissionsLimit *provapipb.EmissionsLimit) error {
		_, err := nextClient().UpdateEmissionsLimit(ctx, &provapipb.UpdateEmissionsLimitRequest{
			EmissionsLimit: emissionsLimit,
			AllowMissing:   false,
		})
		return err
	})
	updateRemoteResources(p, ctx, resourcesInCommon["pointingConstraints"], localResources.pointingConstraints, remoteResources.pointingConstraints, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, pointingConstraint *provapipb.PointingConstraint) error {
		_, err := nextClient().UpdatePointingConstraint(ctx, &provapipb.UpdatePointingConstraintRequest{
			PointingConstraint: pointingConstraint,
			AllowMissing:       false,
		})
		return err
	})

	// Create P2P policies in this pool (candidate paths will be created after).
	createRemoteResources(p, ctx, resourcesToBeAdded["p2pSrTePolicies"], localResources.p2pSrTePolicies, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, policy *provapipb.P2PSrTePolicy) error {
		_, err := nextClient().UpdateP2PSrTePolicy(ctx, &provapipb.UpdateP2PSrTePolicyRequest{
			Policy:       policy,
			AllowMissing: true,
		})
		return err
	})

	createRemoteResources(p, ctx, resourcesToBeAdded["downtimes"], localResources.downtimes, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, downtime *provapipb.Downtime) error {
		_, err := nextClient().UpdateDowntime(ctx, &provapipb.UpdateDowntimeRequest{
			Downtime:     downtime,
			AllowMissing: true,
		})
		return err
	})
	createRemoteResources(p, ctx, resourcesToBeAdded["protectionAssociationGroups"], localResources.protectionAssociationGroups, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, protectionAssociationGroup *provapipb.ProtectionAssociationGroup) error {
		_, err := nextClient().UpdateProtectionAssociationGroup(ctx, &provapipb.UpdateProtectionAssociationGroupRequest{
			ProtectionAssociationGroup: protectionAssociationGroup,
			AllowMissing:               true,
		})
		return err
	})
	createRemoteResources(p, ctx, resourcesToBeAdded["disjointAssociationGroups"], localResources.disjointAssociationGroups, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, disjointAssociationGroup *provapipb.DisjointAssociationGroup) error {
		_, err := nextClient().UpdateDisjointAssociationGroup(ctx, &provapipb.UpdateDisjointAssociationGroupRequest{
			DisjointAssociationGroup: disjointAssociationGroup,
			AllowMissing:             true,
		})
		return err
	})
	createRemoteResources(p, ctx, resourcesToBeAdded["links"], localResources.links, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, link *provapipb.Link) error {
		_, err := nextClient().UpdateLink(ctx, &provapipb.UpdateLinkRequest{
			Link:         link,
			AllowMissing: true,
		})
		return err
	})
	createRemoteResources(p, ctx, resourcesToBeAdded["geographicRegions"], localResources.geographicRegions, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, geographicRegion *provapipb.GeographicRegion) error {
		_, err := nextClient().UpdateGeographicRegion(ctx, &provapipb.UpdateGeographicRegionRequest{
			GeographicRegion: geographicRegion,
			AllowMissing:     true,
		})
		return err
	})
	createRemoteResources(p, ctx, resourcesToBeAdded["emissionsLimits"], localResources.emissionsLimits, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, emissionsLimit *provapipb.EmissionsLimit) error {
		_, err := nextClient().UpdateEmissionsLimit(ctx, &provapipb.UpdateEmissionsLimitRequest{
			EmissionsLimit: emissionsLimit,
			AllowMissing:   true,
		})
		return err
	})
	createRemoteResources(p, ctx, resourcesToBeAdded["pointingConstraints"], localResources.pointingConstraints, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, pointingConstraint *provapipb.PointingConstraint) error {
		_, err := nextClient().UpdatePointingConstraint(ctx, &provapipb.UpdatePointingConstraintRequest{
			PointingConstraint: pointingConstraint,
			AllowMissing:       true,
		})
		return err
	})

	errs = append(errs, p.Wait())

	// Add candidate paths after all policies are created, so parent policies are guaranteed to exist.
	candidatePathPool := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)
	createRemoteResources(candidatePathPool, ctx, resourcesToBeAdded["p2pSrTePolicyCandidatePaths"], localResources.p2pSrTePolicyCandidatePaths, w, printMode, dryRunMode, syncBar.Incr, func(ctx context.Context, path *provapipb.P2PSrTePolicyCandidatePath) error {
		_, err := nextClient().UpdateP2PSrTePolicyCandidatePath(ctx, &provapipb.UpdateP2PSrTePolicyCandidatePathRequest{
			Path:         path,
			AllowMissing: true,
		})
		return err
	})
	errs = append(errs, candidatePathPool.Wait())

	return errors.Join(errs...)
}

func ProvisioningList(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, serviceProvisioning)
	if err != nil {
		return err
	}
	defer conn.Close()

	provisioningClient := provapipb.NewProvisioningClient(conn)
	remoteResources, err := ProvisioningResourcesFromRemote(appCtx.Context, provisioningClient, &progressBar{})
	if err != nil {
		return err
	}

	fmt.Fprintf(appCtx.App.Writer, "\nfound remote resources:\n%s\n", remoteResources.MarshalledString(marshaller))

	return nil
}

type deleteProvisioningParams struct {
	p2pSrTePolicies             []string
	p2pSrTePolicyCandidatePaths []string
	downtimes                   []string
	protectionAssociationGroups []string
	disjointAssociationGroups   []string
	links                       []string
	geographicRegions           []string
	emissionsLimits             []string
	pointingConstraints         []string

	w              io.Writer
	printMode      bool
	dryRunMode     bool
	maxConcurrency int
	nextClient     func() provapipb.ProvisioningClient
	onProgress     func()
}

func deleteProvisioning(ctx context.Context, params deleteProvisioningParams) error {
	printMode := params.printMode
	dryRunMode := params.dryRunMode

	// Delete candidate paths before the policies, in case they need to be removed separately.
	p := pool.New().WithErrors().WithMaxGoroutines(params.maxConcurrency)
	deleteRemoteResources(p, ctx, params.p2pSrTePolicyCandidatePaths, params.w, printMode, dryRunMode, params.onProgress, func(ctx context.Context, path string) error {
		_, err := params.nextClient().DeleteP2PSrTePolicyCandidatePath(ctx, &provapipb.DeleteP2PSrTePolicyCandidatePathRequest{
			Name: path,
		})
		return err
	})
	err := p.Wait()
	if err != nil {
		return err
	}

	p = pool.New().WithErrors().WithMaxGoroutines(params.maxConcurrency)
	deleteRemoteResources(p, ctx, params.p2pSrTePolicies, params.w, printMode, dryRunMode, params.onProgress, func(ctx context.Context, policy string) error {
		_, err := params.nextClient().DeleteP2PSrTePolicy(ctx, &provapipb.DeleteP2PSrTePolicyRequest{
			Name: policy,
		})
		return err
	})
	deleteRemoteResources(p, ctx, params.downtimes, params.w, printMode, dryRunMode, params.onProgress, func(ctx context.Context, downtime string) error {
		_, err := params.nextClient().DeleteDowntime(ctx, &provapipb.DeleteDowntimeRequest{
			Name: downtime,
		})
		return err
	})
	deleteRemoteResources(p, ctx, params.protectionAssociationGroups, params.w, printMode, dryRunMode, params.onProgress, func(ctx context.Context, name string) error {
		_, err := params.nextClient().DeleteProtectionAssociationGroup(ctx, &provapipb.DeleteProtectionAssociationGroupRequest{
			Name: name,
		})
		return err
	})
	deleteRemoteResources(p, ctx, params.disjointAssociationGroups, params.w, printMode, dryRunMode, params.onProgress, func(ctx context.Context, name string) error {
		_, err := params.nextClient().DeleteDisjointAssociationGroup(ctx, &provapipb.DeleteDisjointAssociationGroupRequest{
			Name: name,
		})
		return err
	})
	deleteRemoteResources(p, ctx, params.links, params.w, printMode, dryRunMode, params.onProgress, func(ctx context.Context, link string) error {
		_, err := params.nextClient().DeleteLink(ctx, &provapipb.DeleteLinkRequest{
			Name: link,
		})
		return err
	})
	deleteRemoteResources(p, ctx, params.geographicRegions, params.w, printMode, dryRunMode, params.onProgress, func(ctx context.Context, name string) error {
		_, err := params.nextClient().DeleteGeographicRegion(ctx, &provapipb.DeleteGeographicRegionRequest{
			Name: name,
		})
		return err
	})
	deleteRemoteResources(p, ctx, params.emissionsLimits, params.w, printMode, dryRunMode, params.onProgress, func(ctx context.Context, name string) error {
		_, err := params.nextClient().DeleteEmissionsLimit(ctx, &provapipb.DeleteEmissionsLimitRequest{
			Name: name,
		})
		return err
	})
	deleteRemoteResources(p, ctx, params.pointingConstraints, params.w, printMode, dryRunMode, params.onProgress, func(ctx context.Context, name string) error {
		_, err := params.nextClient().DeletePointingConstraint(ctx, &provapipb.DeletePointingConstraintRequest{
			Name: name,
		})
		return err
	})

	return p.Wait()
}

func newProvisioningClientPool(appCtx *cli.Context, maxConcurrency int) (func() provapipb.ProvisioningClient, func(), error) {
	target, dialOpts, err := resolveAPIDialOpts(appCtx, serviceProvisioning)
	if err != nil {
		return nil, nil, err
	}
	conns := make([]*grpc.ClientConn, maxConcurrency)
	clients := make([]provapipb.ProvisioningClient, maxConcurrency)
	for i := range maxConcurrency {
		conn, err := grpc.NewClient(target, dialOpts...)
		if err != nil {
			for j := range i {
				conns[j].Close()
			}
			return nil, nil, fmt.Errorf("unable to connect to the server: %w", err)
		}
		conns[i] = conn
		clients[i] = provapipb.NewProvisioningClient(conn)
	}
	var clientIdx atomic.Int64
	nextClient := func() provapipb.ProvisioningClient {
		return clients[clientIdx.Add(1)%int64(maxConcurrency)]
	}
	closeAll := func() {
		for _, conn := range conns {
			conn.Close()
		}
	}
	return nextClient, closeAll, nil
}

func ProvisioningDeleteAll(appCtx *cli.Context) error {
	ctx := appCtx.Context
	dryRunMode := !appCtx.Bool("execute")
	verboseMode := appCtx.Bool("verbose")
	printMode := dryRunMode || verboseMode
	maxConcurrency := appCtx.Int("max-concurrency")
	showProgress := shouldShowProgress(appCtx)

	nextClient, closeAll, err := newProvisioningClientPool(appCtx, maxConcurrency)
	if err != nil {
		return err
	}
	defer closeAll()

	progress := newSyncProgress(showProgress)
	listBar := progress.AddBar("listing remote resources", 9)
	progress.Start()
	defer progress.Stop()

	remoteResources, err := ProvisioningResourcesFromRemote(ctx, nextClient(), listBar)
	if err != nil {
		return err
	}

	params := deleteProvisioningParams{
		w:              progress.Writer(),
		printMode:      printMode,
		dryRunMode:     dryRunMode,
		maxConcurrency: maxConcurrency,
		nextClient:     nextClient,
		onProgress:     func() {},

		p2pSrTePolicies:             lo.Keys(remoteResources.p2pSrTePolicies),
		p2pSrTePolicyCandidatePaths: lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths),
		downtimes:                   lo.Keys(remoteResources.downtimes),
		protectionAssociationGroups: lo.Keys(remoteResources.protectionAssociationGroups),
		disjointAssociationGroups:   lo.Keys(remoteResources.disjointAssociationGroups),
		links:                       lo.Keys(remoteResources.links),
		geographicRegions:           lo.Keys(remoteResources.geographicRegions),
		emissionsLimits:             lo.Keys(remoteResources.emissionsLimits),
		pointingConstraints:         lo.Keys(remoteResources.pointingConstraints),
	}

	return deleteProvisioning(appCtx.Context, params)
}

func ProvisioningDelete(appCtx *cli.Context) error {
	ctx := appCtx.Context
	dryRunMode := appCtx.Bool("dry-run")
	verboseMode := appCtx.Bool("verbose")
	resourceNames := appCtx.Args().Slice()
	printMode := dryRunMode || verboseMode
	maxConcurrency := appCtx.Int("max-concurrency")

	if len(resourceNames) == 0 {
		return errors.New("no resource names specified")
	}

	nextClient, closeAll, err := newProvisioningClientPool(appCtx, maxConcurrency)
	if err != nil {
		return err
	}
	defer closeAll()

	remoteResources, err := ProvisioningResourcesFromRemote(ctx, nextClient(), &progressBar{})
	if err != nil {
		return err
	}

	params := deleteProvisioningParams{
		w:              os.Stderr,
		printMode:      printMode,
		dryRunMode:     dryRunMode,
		maxConcurrency: maxConcurrency,
		nextClient:     nextClient,
		onProgress:     func() {},
	}
	params.p2pSrTePolicies = lo.Intersect(resourceNames, lo.Keys(remoteResources.p2pSrTePolicies))
	params.p2pSrTePolicyCandidatePaths = lo.Intersect(resourceNames, lo.Keys(remoteResources.p2pSrTePolicyCandidatePaths))
	params.downtimes = lo.Intersect(resourceNames, lo.Keys(remoteResources.downtimes))
	params.protectionAssociationGroups = lo.Intersect(resourceNames, lo.Keys(remoteResources.protectionAssociationGroups))
	params.disjointAssociationGroups = lo.Intersect(resourceNames, lo.Keys(remoteResources.disjointAssociationGroups))
	params.links = lo.Intersect(resourceNames, lo.Keys(remoteResources.links))
	params.geographicRegions = lo.Intersect(resourceNames, lo.Keys(remoteResources.geographicRegions))
	params.emissionsLimits = lo.Intersect(resourceNames, lo.Keys(remoteResources.emissionsLimits))
	params.pointingConstraints = lo.Intersect(resourceNames, lo.Keys(remoteResources.pointingConstraints))

	deleteResourceNameSet := slices.Concat(
		params.p2pSrTePolicies,
		params.p2pSrTePolicyCandidatePaths,
		params.downtimes,
		params.protectionAssociationGroups,
		params.disjointAssociationGroups,
		params.links,
		params.geographicRegions,
		params.emissionsLimits,
		params.pointingConstraints,
	)

	notFoundNames := lo.Without(resourceNames, deleteResourceNameSet...)

	fmt.Fprintf(os.Stderr, "- %d resources not found\n", len(notFoundNames))
	if verboseMode {
		for _, name := range notFoundNames {
			fmt.Fprintln(os.Stderr, "  -", name)
		}
	}
	fmt.Fprintf(os.Stderr, "- %d resources to be deleted\n", len(deleteResourceNameSet))

	if len(deleteResourceNameSet) == 0 {
		return nil
	}

	return deleteProvisioning(appCtx.Context, params)
}

func createRemoteResources[T proto.Message](p *pool.ErrorPool, ctx context.Context, resourceIDs []string, localResources map[string]T, w io.Writer, printMode bool, dryRunMode bool, onProgress func(), createFn func(context.Context, T) error) {
	for _, key := range resourceIDs {
		p.Go(func() error {
			if printMode {
				fmt.Fprintf(w, "add resource: %s\n", key)
			}
			if !dryRunMode {
				err := withRetry(ctx, func(ctx context.Context) error {
					return createFn(ctx, localResources[key])
				})
				if err != nil {
					return err
				}
			}
			onProgress()
			return nil
		})
	}
}

func updateRemoteResources[T proto.Message](
	p *pool.ErrorPool,
	ctx context.Context,
	resourceIDs []string,
	localResources map[string]T, remoteResources map[string]T,
	w io.Writer, printMode bool, dryRunMode bool,
	onProgress func(),
	updateFn func(context.Context, T) error,
) {
	for _, key := range resourceIDs {
		p.Go(func() error {
			if provisioningResourcesAreEquivalent(localResources[key], remoteResources[key]) {
				onProgress()
				return nil
			}
			if printMode {
				fmt.Fprintf(w, "update resource: %s\n", key)
			}
			if !dryRunMode {
				err := withRetry(ctx, func(ctx context.Context) error {
					return updateFn(ctx, localResources[key])
				})
				if err != nil {
					return err
				}
			}
			onProgress()
			return nil
		})
	}
}

func deleteRemoteResources(p *pool.ErrorPool, ctx context.Context, resourceIDs []string, w io.Writer, printMode bool, dryRunMode bool, onProgress func(), deleteFn func(context.Context, string) error) {
	for _, key := range resourceIDs {
		p.Go(func() error {
			if printMode {
				fmt.Fprintf(w, "delete resource: %s\n", key)
			}
			if !dryRunMode {
				err := withRetry(ctx, func(ctx context.Context) error {
					return deleteFn(ctx, key)
				})
				if err != nil && !isNotFoundError(err) {
					return err
				}
			}
			onProgress()
			return nil
		})
	}
}
